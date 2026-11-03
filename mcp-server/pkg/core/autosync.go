package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAutoSyncInterval is how stale a project's index may get before a
// search re-syncs it from disk.
const DefaultAutoSyncInterval = 60 * time.Second

// autoSyncTimeout bounds how long one search waits for a re-sync. A sync
// cut short keeps what it indexed (unchanged chunks are skipped next time),
// so a big change is caught up over the next searches.
const autoSyncTimeout = 30 * time.Second

// DirRegistry remembers which directories were indexed into which project,
// in a JSON file, so a later process (every stdio session is one) can keep
// those directories in sync without being told again. Several processes may
// share the file (agent sessions, the dashboard), so it is re-read before
// every read and every change, and written atomically.
type DirRegistry struct {
	mu   sync.Mutex
	path string
	dirs map[string][]string // project → absolute directories
}

// NewDirRegistry opens the registry stored at path (made on first write).
func NewDirRegistry(path string) (*DirRegistry, error) {
	r := &DirRegistry{path: path, dirs: map[string][]string{}}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// load re-reads the file, so changes by other processes are seen. A missing
// file is an empty registry. Caller holds mu (or owns r).
func (r *DirRegistry) load() error {
	raw, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		r.dirs = map[string][]string{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("project dirs: %w", err)
	}
	dirs := map[string][]string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &dirs); err != nil {
			return fmt.Errorf("project dirs %s: %w", r.path, err)
		}
	}
	r.dirs = dirs
	return nil
}

// Add records dir (made absolute) as indexed into project.
func (r *DirRegistry) Add(project, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.load(); err != nil {
		return err
	}
	for _, d := range r.dirs[project] {
		if d == abs {
			return nil
		}
	}
	r.dirs[project] = append(r.dirs[project], abs)
	sort.Strings(r.dirs[project])
	return r.save()
}

// Remove forgets every directory of project.
func (r *DirRegistry) Remove(project string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.load(); err != nil {
		return err
	}
	if _, ok := r.dirs[project]; !ok {
		return nil
	}
	delete(r.dirs, project)
	return r.save()
}

// Dirs returns the directories indexed into project, as the file has them
// now (if it can't be read, as last read).
func (r *DirRegistry) Dirs(project string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.load(); err != nil {
		fmt.Fprintf(os.Stderr, "auto-sync: %v\n", err)
	}
	return append([]string(nil), r.dirs[project]...)
}

// save writes the registry atomically (temp file + rename). Caller holds mu.
func (r *DirRegistry) save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r.dirs, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".project_dirs-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}

// autoSync holds the Service's auto-sync state.
type autoSync struct {
	registry *DirRegistry
	every    time.Duration

	mu    sync.Mutex
	last  map[string]time.Time   // project\x00dir → last sync
	locks map[string]*sync.Mutex // project\x00dir → one sync at a time
}

// EnableAutoSync makes the Service keep indexed directories in sync: every
// directory indexed with IndexProject is recorded in registry, and a search
// on a project first re-syncs its directories whose last sync is older than
// every (incremental: only changed chunks are embedded). every <= 0, or a nil
// registry, disables it.
func (s *Service) EnableAutoSync(registry *DirRegistry, every time.Duration) {
	if registry == nil || every <= 0 {
		s.syncer = nil
		return
	}
	s.syncer = &autoSync{registry: registry, every: every,
		last: map[string]time.Time{}, locks: map[string]*sync.Mutex{}}
}

// lock returns the mutex serializing syncs of one project directory.
func (a *autoSync) lock(key string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	l, ok := a.locks[key]
	if !ok {
		l = &sync.Mutex{}
		a.locks[key] = l
	}
	return l
}

func (a *autoSync) lastSync(key string) time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.last[key]
}

func (a *autoSync) markSynced(key string) {
	a.mu.Lock()
	a.last[key] = time.Now()
	a.mu.Unlock()
}

func syncKey(project, dir string) string { return project + "\x00" + dir }

// checkDirNameClash returns an error when project already has a recorded
// directory with dir's base name but another path (see IndexProject).
func (s *Service) checkDirNameClash(project, dir string) error {
	if s.syncer == nil {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	for _, d := range s.syncer.registry.Dirs(project) {
		if d != abs && filepath.Base(d) == filepath.Base(abs) {
			return fmt.Errorf("project %q already indexes %s, which has the same folder name as %s: "+
				"their chunks would replace each other; index it into a separate project", project, d, abs)
		}
	}
	return nil
}

// recordIndexed notes a successful IndexProject of dir into project.
func (s *Service) recordIndexed(project, dir string) {
	if s.syncer == nil {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	if err := s.syncer.registry.Add(project, abs); err != nil {
		fmt.Fprintf(os.Stderr, "auto-sync: could not record %s: %v\n", abs, err)
	}
	s.syncer.markSynced(syncKey(project, abs))
}

// autoSyncProject re-syncs project's recorded directories that are due,
// before a search reads the index. Failures never fail the search: they are
// logged, and the directory is tried again after the interval. Skipped, never
// synced: a directory that no longer exists (that would wipe its index) and a
// project that was deleted (that would bring it back). Directories of one
// project sharing a base name are refused by IndexProject itself.
func (s *Service) autoSyncProject(ctx context.Context, project string) {
	a := s.syncer
	if a == nil {
		return
	}
	checkedExists := false
	for _, dir := range a.registry.Dirs(project) {
		key := syncKey(project, dir)
		l := a.lock(key)
		l.Lock()
		if time.Since(a.lastSync(key)) < a.every {
			l.Unlock()
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			a.markSynced(key) // don't stat it on every search
			l.Unlock()
			continue
		}
		if !checkedExists {
			if !s.ProjectExists(ctx, project) {
				l.Unlock()
				return // deleted (maybe by another process): don't recreate it
			}
			checkedExists = true
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), autoSyncTimeout)
		if _, err := s.IndexProject(sctx, project, dir); err != nil {
			fmt.Fprintf(os.Stderr, "auto-sync %s (%s): %v\n", project, dir, err)
		}
		cancel()
		a.markSynced(key)
		l.Unlock()
	}
}

// ParseAutoSyncInterval reads the auto-sync interval (config file
// auto_sync_interval, or env RAG_AUTO_SYNC_INTERVAL): a duration ("90s",
// "5m"), a whole number of seconds, or "0" / "off" / "false" to disable.
// Empty means DefaultAutoSyncInterval; anything else is an error.
func ParseAutoSyncInterval(v string) (time.Duration, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "":
		return DefaultAutoSyncInterval, nil
	case "0", "off", "false", "no", "disabled":
		return 0, nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("auto-sync interval %q: negative", v)
		}
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("auto-sync interval %q: want a duration like 60s, seconds, or off", v)
	}
	if d < 0 {
		return 0, fmt.Errorf("auto-sync interval %q: negative", v)
	}
	return d, nil
}
