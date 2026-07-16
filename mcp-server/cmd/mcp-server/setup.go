package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runSetup implements the `enowx-rag setup` subcommand. It generates a
// docker-compose file for the configured backend and prints the commands to
// start it. With --run it executes `docker compose up -d` for the required
// services directly in the user's terminal.
//
// Execution is intentionally CLI-only: the web UI never runs docker, so there
// is no remote command-execution surface on the HTTP server.
func runSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	run := fs.Bool("run", false, "run `docker compose up -d` for the configured backend")
	file := fs.String("file", "docker-compose.enowx.yml", "path to write the generated compose file")
	_ = fs.Parse(args)

	cfg, err := resolveConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve config: %v\n", err)
		os.Exit(1)
	}

	compose := generateCompose(cfg)
	services := composeServices(cfg)

	if !*run {
		// Print-only mode: show the compose file and the commands to run.
		fmt.Printf("# Generated compose for vector_store=%s embedder=%s\n\n", cfg.VectorStore, cfg.Embedder)
		fmt.Println(compose)
		fmt.Printf("\n# To start the backend, run:\n")
		fmt.Printf("docker compose -f %s up -d %s\n", *file, strings.Join(services, " "))
		fmt.Printf("\n# Or let enowx-rag do it for you:\n")
		fmt.Printf("enowx-rag setup --run\n")
		return
	}

	// --run: write the compose file and execute docker compose up -d.
	if err := os.WriteFile(*file, []byte(compose), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *file, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *file)

	cmdArgs := append([]string{"compose", "-f", *file, "up", "-d"}, services...)
	fmt.Fprintf(os.Stderr, "running: docker %s\n", strings.Join(cmdArgs, " "))
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "docker compose failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "backend started. Start the server with: enowx-rag --serve\n")
}

// composeServices returns the docker-compose service names required by the
// configured vector store and embedder.
func composeServices(cfg *RuntimeConfig) []string {
	var svc []string
	switch strings.ToLower(cfg.VectorStore) {
	case "pgvector":
		svc = append(svc, "postgres")
	case "qdrant":
		svc = append(svc, "qdrant")
	case "chroma":
		svc = append(svc, "chroma")
	}
	if strings.ToLower(cfg.Embedder) == "tei" {
		svc = append(svc, "tei-embedding")
	}
	return svc
}

// generateCompose builds a docker-compose YAML for the configured backend.
// Kept in parity with web/src/pages/onboarding/types.ts generateDockerCompose.
func generateCompose(cfg *RuntimeConfig) string {
	var services, volumes []string

	switch strings.ToLower(cfg.VectorStore) {
	case "pgvector":
		services = append(services, `  postgres:
    image: pgvector/pgvector:pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: enowxrag
      POSTGRES_USER: enowdev
    volumes:
      - pgdata:/var/lib/postgresql/data`)
		volumes = append(volumes, "  pgdata:")
	case "qdrant":
		services = append(services, `  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage`)
		volumes = append(volumes, "  qdrant_data:")
	case "chroma":
		services = append(services, `  chroma:
    image: chromadb/chroma:latest
    ports:
      - "8000:8000"
    volumes:
      - chroma_data:/chroma/chroma`)
		volumes = append(volumes, "  chroma_data:")
	}

	if strings.ToLower(cfg.Embedder) == "tei" {
		services = append(services, `  tei-embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.5
    ports:
      - "8081:80"
    volumes:
      - tei_data:/data`)
		volumes = append(volumes, "  tei_data:")
	}

	out := "services:\n" + strings.Join(services, "\n\n") + "\n"
	if len(volumes) > 0 {
		out += "\nvolumes:\n" + strings.Join(volumes, "\n") + "\n"
	}
	return out
}
