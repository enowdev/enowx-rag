package indexer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/enowdev/enowx-rag/mcp-server/pkg/rag"
)

// Indexer scans a project directory and syncs its contents to RAG.
type Indexer struct {
	provider  rag.Provider
	chunkSize int
	// Files, when set, lists the files to index in a folder (relative,
	// with slashes) in place of git's (or, outside a repository, a walk).
	Files func(root string) ([]string, error)
}

// NewIndexer creates an indexer with the given provider.
// chunkSize is the max characters per chunk (default 1000). The default is
// chosen so a chunk plus its "File: <path>" prefix stays under the TEI
// embedding model's 512-token input limit (code averages ~3 chars/token).
func NewIndexer(provider rag.Provider, chunkSize int) *Indexer {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	return &Indexer{provider: provider, chunkSize: chunkSize}
}

// Default ignore directories.
var defaultIgnores = map[string]bool{
	"node_modules":  true,
	".git":          true,
	"vendor":        true,
	"dist":          true,
	"build":         true,
	".next":         true,
	".nuxt":         true,
	"__pycache__":   true,
	".cache":        true,
	"target":        true,
	".idea":         true,
	".vscode":       true,
	".DS_Store":     true,
	"coverage":      true,
	".pytest_cache": true,
}

// chunkVersion tags the chunk layout. v4: chunk IDs are derived from the
// chunk's content ("dir/file#<hash>") instead of its position in the file.
const chunkVersion = "v4"

// indexBatch bounds how many documents go to the provider per call. The TEI
// embedding client sub-batches these further to respect its own limit.
const indexBatch = 100

// IndexProject scans the directory and syncs its code/text files to the
// project collection, so the stored chunks mirror the files as they are now:
//
//   - A chunk's ID is derived from its text ("dir/file#<content hash>"), so a
//     chunk keeps its ID when code is added or removed around it.
//   - An unchanged chunk at the same lines is skipped (no re-embedding).
//   - An unchanged chunk whose line numbers moved gets its header and line
//     metadata refreshed, without re-embedding when the provider implements
//     rag.PayloadUpdater (re-indexed otherwise).
//   - Every previously indexed chunk of this directory that the scan did not
//     produce again is deleted: chunks of removed files, of code removed from
//     a file that still exists, and chunks written by older chunk layouts.
//
// Points without a source_file (memory notes written with rag_index) are
// never touched. A root that does not exist is an error, so a typo or a moved
// repository cannot wipe the index.
func (idx *Indexer) IndexProject(ctx context.Context, projectID, rootDir string) (*SyncResult, error) {
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(rootDir); err != nil {
		return nil, fmt.Errorf("project directory: %w", err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("project directory %s is not a directory", rootDir)
	}

	// Use the directory base name as a source_dir tag so stale-point
	// reconciliation is scoped per-directory. Without this, indexing a second
	// directory into the same project would delete all points from the first
	// directory because their source_file values don't appear in the second
	// directory's file list.
	sourceDir := filepath.Base(rootDir)

	// Existing points of this source_dir, keyed by their document ID, to skip
	// unchanged chunks and find the stale ones. If they can't be listed, every
	// chunk is indexed and stale cleanup is skipped (reported in StaleError).
	// Only IDs under "<sourceDir>/" are this directory's: not every provider
	// applies the source_dir filter (pgvector filters by source_file only), and
	// another directory's chunks must never be taken for stale ones.
	existing := make(map[string]rag.PointInfo)
	existingPoints, listErr := idx.provider.ListPoints(ctx, projectID, map[string]string{"source_dir": sourceDir})
	if listErr == nil {
		for _, pt := range existingPoints {
			key := pt.DocID
			if key == "" {
				key = pt.ID
			}
			if strings.HasPrefix(key, sourceDir+"/") {
				existing[key] = pt
			}
		}
	}

	// The files: the caller's, else git's (its ignores honored), else a
	// walk; never one that holds secrets.
	var fresh, moved []rag.Document
	keep := make(map[string]bool)       // doc IDs produced by this scan
	unreadable := make(map[string]bool) // files that exist but couldn't be read: keep their chunks
	var currentFiles []string
	skipped := 0
	files, err := idx.listFiles(rootDir)
	if err != nil {
		return nil, err
	}
	for _, relPath := range files {
		if IsSecret(relPath) || !isIndexable(filepath.Base(relPath)) {
			continue
		}
		path := filepath.Join(rootDir, filepath.FromSlash(relPath))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > 500*1024 {
			continue
		}
		currentFiles = append(currentFiles, relPath)
		content, err := os.ReadFile(path)
		if err != nil {
			unreadable[relPath] = true // maybe transient: don't drop what we have
			continue
		}
		if bytes.IndexByte(content, 0) >= 0 {
			continue // binary
		}
		seen := make(map[string]int) // content hash -> occurrences in this file
		for i, ch := range chunkCode(string(content), relPath, idx.chunkSize) {
			if secretText.MatchString(ch.Text) {
				continue // a key in the code: never sent to an embedder
			}
			contentHash := computeContentHash(ch.Text)
			seen[contentHash]++
			docID := fmt.Sprintf("%s/%s#%s", sourceDir, relPath, contentHash)
			if n := seen[contentHash]; n > 1 {
				docID = fmt.Sprintf("%s-%d", docID, n) // identical chunks in one file
			}
			keep[docID] = true

			head := fmt.Sprintf("File: %s:%d-%d", relPath, ch.Start, ch.End)
			if ch.Symbol != "" {
				head += " (" + ch.Symbol + ")"
			}
			doc := rag.Document{
				ID:      docID,
				Content: head + "\n\n" + ch.Text,
				Meta: map[string]string{
					"source_file":   relPath,
					"source_dir":    sourceDir,
					"chunk_index":   fmt.Sprintf("%d", i),
					"content_hash":  contentHash,
					"chunk_version": chunkVersion,
					"start_line":    fmt.Sprintf("%d", ch.Start),
					"end_line":      fmt.Sprintf("%d", ch.End),
					"symbol":        ch.Symbol,
				},
			}
			if pt, ok := existing[docID]; ok && pt.ContentHash == contentHash {
				if sameHeader(pt.Content, head) {
					skipped++
					continue
				}
				moved = append(moved, doc) // same code, new line numbers
				continue
			}
			fresh = append(fresh, doc)
		}
	}

	// Refresh moved chunks without re-embedding when the provider can; any
	// batch it can't update is re-indexed instead.
	updated := 0
	if len(moved) > 0 {
		if up, ok := idx.provider.(rag.PayloadUpdater); ok {
			for start := 0; start < len(moved); start += indexBatch {
				batch := moved[start:min(start+indexBatch, len(moved))]
				if err := up.UpdatePayload(ctx, projectID, batch); err != nil {
					fresh = append(fresh, batch...)
					continue
				}
				updated += len(batch)
			}
		} else {
			fresh = append(fresh, moved...)
		}
	}

	// Index new/changed documents.
	for start := 0; start < len(fresh); start += indexBatch {
		batch := fresh[start:min(start+indexBatch, len(fresh))]
		if err := idx.provider.Index(ctx, projectID, batch); err != nil {
			return nil, fmt.Errorf("index batch %d: %w", start/indexBatch, err)
		}
	}

	result := &SyncResult{
		Indexed:      len(fresh),
		Updated:      updated,
		FilesScanned: len(currentFiles),
		Skipped:      skipped,
	}
	if listErr != nil {
		result.StaleError = fmt.Sprintf("list existing points: %v", listErr)
		return result, nil
	}

	// Delete every chunk of this directory the scan did not produce again.
	var staleIDs []string
	for key, pt := range existing {
		if pt.SourceFile == "" || unreadable[pt.SourceFile] {
			continue // a memory note, or a file we couldn't read this time
		}
		if !keep[key] {
			staleIDs = append(staleIDs, pt.ID)
		}
	}
	if len(staleIDs) > 0 {
		if err := idx.provider.DeletePoints(ctx, projectID, staleIDs); err != nil {
			result.StaleError = err.Error()
			return result, nil
		}
	}
	result.Deleted = len(staleIDs)
	return result, nil
}

// sameHeader reports whether a stored chunk's content (possibly a truncated
// preview) starts with the given header line. Providers that return no
// content preview can't be checked; the chunk is then treated as unchanged,
// since its ID and content hash already match.
func sameHeader(stored, head string) bool {
	if stored == "" {
		return true
	}
	line := head + "\n"
	if len(stored) < len(line) {
		return strings.HasPrefix(line, stored) // preview cut inside the header
	}
	return strings.HasPrefix(stored, line)
}

// listFiles are the files of rootDir to look at, relative with slashes.
func (idx *Indexer) listFiles(rootDir string) ([]string, error) {
	if idx.Files != nil {
		return idx.Files(rootDir)
	}
	if files, ok := gitFiles(rootDir); ok {
		return files, nil
	}
	var files []string
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			if defaultIgnores[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(rootDir, path); err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files, err
}

// SyncResult holds stats from a project sync.
type SyncResult struct {
	Indexed      int    `json:"indexed"`           // chunks embedded and stored
	Updated      int    `json:"updated,omitempty"` // moved chunks refreshed without re-embedding
	Deleted      int    `json:"deleted"`           // stale chunks removed
	FilesScanned int    `json:"files_scanned"`
	Skipped      int    `json:"skipped"` // unchanged chunks
	StaleError   string `json:"stale_error,omitempty"`
}

// chunkText splits text into chunks of approximately maxChars.
// It tries to split on newline boundaries.
func chunkText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	var chunks []string
	lines := strings.Split(text, "\n")
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
	}

	for _, line := range lines {
		// A single line longer than maxChars (e.g. minified/generated code)
		// cannot fit in any chunk on its own, so hard-split it on byte
		// boundaries. Without this the line would be emitted whole and exceed
		// the embedding model's token limit.
		for len(line) > maxChars {
			flush()
			chunks = append(chunks, line[:maxChars])
			line = line[maxChars:]
		}
		if current.Len()+len(line)+1 > maxChars && current.Len() > 0 {
			flush()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	flush()
	return chunks
}

// computeContentHash returns the first 8 bytes of SHA-256 of the given content
// as a 16-character lowercase hex string. This is used for incremental sync:
// chunks whose hash matches an existing point are skipped (no re-embedding).
func computeContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8])
}

// isIndexable returns true for code and text files.
func isIndexable(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	codeExts := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true, ".cs": true,
		".rb": true, ".php": true, ".vue": true, ".svelte": true,
		".sql": true, ".sh": true, ".bash": true, ".zsh": true,
		".yml": true, ".yaml": true, ".toml": true, ".json": true,
		".xml": true, ".html": true, ".css": true, ".scss": true,
		".md": true, ".txt": true, ".cfg": true,
		".ini": true, ".conf": true, ".dockerfile": true,
		".proto": true, ".graphql": true, ".gql": true,
	}
	if codeExts[ext] {
		return true
	}
	// Also index files without extension but with known names
	nameLower := strings.ToLower(name)
	if nameLower == "dockerfile" || nameLower == "makefile" || nameLower == "license" || nameLower == ".gitignore" {
		return true
	}
	return false
}
