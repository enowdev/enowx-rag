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

	"github.com/enowdev/enowx-rag/pkg/rag"
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

// IndexProject scans the directory and indexes all code/text files.
// It handles insertions (new/changed files), and deletions (files removed since last index).
// Chunks whose content_hash matches an existing point are skipped (no re-embedding).
func (idx *Indexer) IndexProject(ctx context.Context, projectID, rootDir string) (*SyncResult, error) {
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Use the directory base name as a source_dir tag so stale-point
	// reconciliation is scoped per-directory. Without this, indexing a second
	// directory into the same project would delete all points from the first
	// directory because their source_file values don't appear in the second
	// directory's file list.
	sourceDir := filepath.Base(rootDir)

	// Fetch existing points for this source_dir so we can compare content hashes
	// and skip re-embedding unchanged chunks.
	existingPoints, err := idx.provider.ListPoints(ctx, projectID, map[string]string{"source_dir": sourceDir})
	if err != nil {
		// If we can't list existing points, proceed without skip optimization
		existingPoints = nil
	}
	existingHashes := make(map[string]string) // docID -> content_hash
	for _, pt := range existingPoints {
		key := pt.DocID
		if key == "" {
			key = pt.ID
		}
		if pt.ContentHash != "" {
			existingHashes[key] = pt.ContentHash
		}
	}

	// The files: the caller's, else git's (its ignores honored), else a
	// walk; never one that holds secrets.
	var docs []rag.Document
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
		if err != nil || bytes.IndexByte(content, 0) >= 0 {
			continue // unreadable, or binary
		}
		for i, ch := range chunkCode(string(content), relPath, idx.chunkSize) {
			if secretText.MatchString(ch.Text) {
				continue // a key in the code: never sent to an embedder
			}
			docID := fmt.Sprintf("%s/%s#chunk%d", sourceDir, relPath, i)
			contentHash := computeContentHash(ch.Text)
			if existingHash, ok := existingHashes[docID]; ok && existingHash == contentHash {
				skipped++
				continue
			}
			head := fmt.Sprintf("File: %s:%d-%d", relPath, ch.Start, ch.End)
			if ch.Symbol != "" {
				head += " (" + ch.Symbol + ")"
			}
			docs = append(docs, rag.Document{
				ID:      docID,
				Content: head + "\n\n" + ch.Text,
				Meta: map[string]string{
					"source_file":   relPath,
					"source_dir":    sourceDir,
					"chunk_index":   fmt.Sprintf("%d", i),
					"content_hash":  contentHash,
					"chunk_version": "v3",
					"start_line":    fmt.Sprintf("%d", ch.Start),
					"end_line":      fmt.Sprintf("%d", ch.End),
					"symbol":        ch.Symbol,
				},
			})
		}
	}

	// Index new/changed documents.
	if len(docs) > 0 {
		// Batch index 100 docs per provider call to bound request size. The TEI
		// embedding client sub-batches these further to respect its own limit.
		for i := 0; i < len(docs); i += 100 {
			end := i + 100
			if end > len(docs) {
				end = len(docs)
			}
			if err := idx.provider.Index(ctx, projectID, docs[i:end]); err != nil {
				return nil, fmt.Errorf("index batch %d: %w", i/100, err)
			}
		}
	}

	// Find and delete stale points (files that no longer exist in THIS directory).
	staleIDs, err := idx.findStalePoints(ctx, projectID, sourceDir, currentFiles)
	if err != nil {
		return &SyncResult{Indexed: len(docs), Deleted: 0, Skipped: skipped, StaleError: err.Error()}, nil
	}
	if len(staleIDs) > 0 {
		if err := idx.provider.DeletePoints(ctx, projectID, staleIDs); err != nil {
			return &SyncResult{Indexed: len(docs), Deleted: 0, Skipped: skipped, StaleError: err.Error()}, nil
		}
	}

	return &SyncResult{
		Indexed:      len(docs),
		Deleted:      len(staleIDs),
		FilesScanned: len(currentFiles),
		Skipped:      skipped,
	}, nil
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
	Indexed      int    `json:"indexed"`
	Deleted      int    `json:"deleted"`
	FilesScanned int    `json:"files_scanned"`
	Skipped      int    `json:"skipped"`
	StaleError   string `json:"stale_error,omitempty"`
}

func (idx *Indexer) findStalePoints(ctx context.Context, projectID, sourceDir string, currentFiles []string) ([]string, error) {
	currentSet := make(map[string]bool, len(currentFiles))
	for _, f := range currentFiles {
		currentSet[f] = true
	}

	// List only points belonging to this source_dir so that indexing a
	// different directory into the same project doesn't wipe the first.
	points, err := idx.provider.ListPoints(ctx, projectID, map[string]string{"source_dir": sourceDir})
	if err != nil {
		return nil, err
	}

	var stale []string
	for _, pt := range points {
		if pt.SourceFile == "" {
			continue
		}
		if !currentSet[pt.SourceFile] {
			stale = append(stale, pt.ID)
		}
	}
	return stale, nil
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
