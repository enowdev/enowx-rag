package indexer

import (
	"fmt"
	"strings"
	"testing"
)

// body returns n indented filler lines at the given indent.
func body(indent string, n int, tag string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%sx_%d = compute(%q, %d)\n", indent, i, tag, i)
	}
	return b.String()
}

func symbols(chunks []Chunk) []string {
	var out []string
	for _, c := range chunks {
		out = append(out, c.Symbol)
	}
	return out
}

func chunkWith(t *testing.T, chunks []Chunk, sym string) Chunk {
	t.Helper()
	for _, c := range chunks {
		if c.Symbol == sym {
			return c
		}
	}
	t.Fatalf("no chunk with symbol %q in %q", sym, symbols(chunks))
	return Chunk{}
}

// A Python class too long for one chunk is split at its methods: each chunk
// starts at a method (decorators kept with it), named Class.method, and a
// function nested in a method stays inside that method.
func TestChunkCode_PythonClassSplitAtMethods(t *testing.T) {
	src := "class UserService:\n" +
		"    \"\"\"Users.\"\"\"\n\n" +
		"    def __init__(self, db):\n" + body("        ", 8, "init") + "\n" +
		"    @property\n" +
		"    def active(self):\n" + body("        ", 8, "active") + "\n" +
		"    async def save(self, user):\n" +
		"        def validate(u):\n" + body("            ", 3, "validate") +
		body("        ", 6, "save")
	chunks := chunkCode(src, "svc.py", 400)

	active := chunkWith(t, chunks, "UserService.active")
	if !strings.HasPrefix(active.Text, "    @property\n    def active") {
		t.Errorf("decorator not kept with its method:\n%s", active.Text)
	}
	save := chunkWith(t, chunks, "UserService.save")
	if !strings.Contains(save.Text, "def validate") {
		t.Errorf("nested function split out of its method:\n%s", save.Text)
	}
	for _, c := range chunks {
		if c.Symbol == "UserService.validate" {
			t.Errorf("nested function became its own chunk: %q", symbols(chunks))
		}
		if len(c.Text) > 400 {
			t.Errorf("chunk over the size (%d): %s", len(c.Text), c.Symbol)
		}
	}
	// Line numbers stay those of the file.
	lines := strings.Split(src, "\n")
	if !strings.HasPrefix(lines[active.Start-1], "    @property") {
		t.Errorf("active chunk starts at line %d: %q", active.Start, lines[active.Start-1])
	}
}

// Java: methods and constructors with modifiers are members; a call inside
// a method body is not; annotations stay with their method.
func TestChunkCode_JavaClassSplitAtMethods(t *testing.T) {
	src := "public class OrderService {\n" +
		"    private final Repo repo;\n\n" +
		"    public OrderService(Repo repo) {\n" + body("        ", 6, "ctor") + "    }\n\n" +
		"    @Override\n" +
		"    public List<Order> findAll() {\n" + body("        ", 6, "find") +
		"        helper(repo);\n" +
		"    }\n\n" +
		"    private static <T> T pick(List<T> xs) {\n" + body("        ", 6, "pick") + "    }\n" +
		"}\n"
	chunks := chunkCode(src, "OrderService.java", 350)
	chunkWith(t, chunks, "OrderService.OrderService")
	find := chunkWith(t, chunks, "OrderService.findAll")
	if !strings.HasPrefix(strings.TrimSpace(find.Text), "@Override") {
		t.Errorf("annotation not kept with method:\n%s", find.Text)
	}
	chunkWith(t, chunks, "OrderService.pick")
	for _, s := range symbols(chunks) {
		if s == "OrderService.helper" {
			t.Errorf("a call was taken for a method: %q", symbols(chunks))
		}
	}
}

// TypeScript: class methods are members; control flow ("if (x) {") is not.
func TestChunkCode_TypeScriptClassSplitAtMethods(t *testing.T) {
	src := "export class Cart {\n" +
		"  private items: Item[] = [];\n\n" +
		"  async load(id: string): Promise<void> {\n" + body("    ", 6, "load") +
		"    if (id) {\n" + body("      ", 2, "if") + "    }\n" +
		"  }\n\n" +
		"  get total(): number {\n" + body("    ", 6, "total") + "  }\n" +
		"}\n"
	chunks := chunkCode(src, "cart.ts", 300)
	load := chunkWith(t, chunks, "Cart.load")
	if !strings.Contains(load.Text, "if (id) {") {
		t.Errorf("if block split out of its method:\n%s", load.Text)
	}
	chunkWith(t, chunks, "Cart.total")
	for _, s := range symbols(chunks) {
		if s == "Cart.if" {
			t.Errorf("control flow taken for a method: %q", symbols(chunks))
		}
	}
}

// Rust: fns in an impl block are members.
func TestChunkCode_RustImplSplitAtFns(t *testing.T) {
	src := "impl Server {\n" +
		"    pub fn new(addr: &str) -> Self {\n" + body("        ", 6, "new") + "    }\n\n" +
		"    /// Starts it.\n" +
		"    pub async fn start(&self) {\n" + body("        ", 6, "start") + "    }\n" +
		"}\n"
	chunks := chunkCode(src, "server.rs", 300)
	chunkWith(t, chunks, "Server.new")
	start := chunkWith(t, chunks, "Server.start")
	if !strings.HasPrefix(start.Text, "    /// Starts it.") {
		t.Errorf("doc comment not kept with fn:\n%s", start.Text)
	}
}

// A class that fits in one chunk is not split; a long declaration in a
// language without members known is still cut at blank lines.
func TestChunkCode_MembersOnlyWhenTooLong(t *testing.T) {
	src := "class Small:\n    def a(self):\n        return 1\n\n    def b(self):\n        return 2\n"
	if got := chunkCode(src, "s.py", 1000); len(got) != 1 || got[0].Symbol != "Small" {
		t.Fatalf("small class split: %+v", got)
	}
	goSrc := "func Big() {\n" + body("\t", 10, "a") + "\n" + body("\t", 10, "b") + "}\n"
	got := chunkCode(goSrc, "big.go", 300)
	if len(got) < 2 {
		t.Fatalf("long Go func not cut: %+v", got)
	}
	for _, c := range got {
		if c.Symbol != "Big" || len(c.Text) > 300 {
			t.Fatalf("cut piece: %+v", c)
		}
	}
}
