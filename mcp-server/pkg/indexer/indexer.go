package indexer

import (
	"context"
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
}

// NewIndexer creates an indexer with the given provider.
// chunkSize is the max characters per chunk (default 1500).
func NewIndexer(provider rag.Provider, chunkSize int) *Indexer {
	if chunkSize <= 0 {
		chunkSize = 1500
	}
	return &Indexer{provider: provider, chunkSize: chunkSize}
}

// Default ignore directories.
var defaultIgnores = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".nuxt":        true,
	"__pycache__":  true,
	".cache":       true,
	"target":       true,
	".idea":        true,
	".vscode":      true,
	".DS_Store":    true,
	"coverage":     true,
	".pytest_cache": true,
}

// IndexProject scans the directory and indexes all code/text files.
// It handles insertions (new/changed files), and deletions (files removed since last index).
func (idx *Indexer) IndexProject(ctx context.Context, projectID, rootDir string) (*SyncResult, error) {
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	// Walk the directory and collect files.
	var docs []rag.Document
	var currentFiles []string

	err = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			name := d.Name()
			if defaultIgnores[name] {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip binary/irrelevant files
		if !isIndexable(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		currentFiles = append(currentFiles, relPath)

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Skip empty or very large files (>500KB)
		if len(content) == 0 || len(content) > 500*1024 {
			return nil
		}

		chunks := chunkText(string(content), idx.chunkSize)
		for i, chunk := range chunks {
			docID := fmt.Sprintf("%s#chunk%d", relPath, i)
			docs = append(docs, rag.Document{
				ID:      docID,
				Content: fmt.Sprintf("File: %s\n\n%s", relPath, chunk),
				Meta: map[string]string{
					"source_file": relPath,
					"chunk_index": fmt.Sprintf("%d", i),
				},
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Index new/changed documents.
	if len(docs) > 0 {
		// Batch index (max 100 at a time to avoid huge requests)
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

	// Find and delete stale points (files that no longer exist).
	staleIDs, err := idx.findStalePoints(ctx, projectID, currentFiles)
	if err != nil {
		return &SyncResult{Indexed: len(docs), Deleted: 0, StaleError: err.Error()}, nil
	}
	if len(staleIDs) > 0 {
		if err := idx.provider.DeletePoints(ctx, projectID, staleIDs); err != nil {
			return &SyncResult{Indexed: len(docs), Deleted: 0, StaleError: err.Error()}, nil
		}
	}

	return &SyncResult{
		Indexed:       len(docs),
		Deleted:       len(staleIDs),
		FilesScanned:  len(currentFiles),
	}, nil
}

// SyncResult holds stats from a project sync.
type SyncResult struct {
	Indexed      int    `json:"indexed"`
	Deleted      int    `json:"deleted"`
	FilesScanned int    `json:"files_scanned"`
	StaleError   string `json:"stale_error,omitempty"`
}

func (idx *Indexer) findStalePoints(ctx context.Context, projectID string, currentFiles []string) ([]string, error) {
	currentSet := make(map[string]bool, len(currentFiles))
	for _, f := range currentFiles {
		currentSet[f] = true
	}

	// List all points that have source_file metadata.
	allIDs, err := idx.provider.ListPointIDs(ctx, projectID, nil)
	if err != nil {
		return nil, err
	}

	// We need to also know which source_file each point belongs to.
	// Since ListPointIDs only returns IDs, we need to check by file.
	// Strategy: for each file that was previously indexed but is now gone, delete its points.
	// We can't easily get source_file from ListPointIDs alone, so we use a different approach:
	// list all points, then for each, check if its source_file is in currentSet.
	// But that requires payload. Let's use a simpler approach: just re-index everything.
	// The upsert will overwrite existing points with the same ID.
	// For deletion, we need to know which files were removed.
	// Since point IDs contain the file path (format: "path/to/file.go#chunk0"),
	// we can extract source_file from the ID.

	var stale []string
	for _, id := range allIDs {
		// ID format: "path/to/file.go#chunk0"
		parts := strings.Split(id, "#")
		if len(parts) < 2 {
			continue
		}
		sourceFile := parts[0]
		if !currentSet[sourceFile] {
			stale = append(stale, id)
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

	for _, line := range lines {
		if current.Len()+len(line)+1 > maxChars && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
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
		".md": true, ".txt": true, ".env": true, ".cfg": true,
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
