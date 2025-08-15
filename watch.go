package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	mbtiles "github.com/brendan-ward/mbtiles-go"
	"github.com/consbio/mbtileserver/handlers"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"
)

// debounce debounces requests to a callback function to occur no more
// frequently than interval; once this is reached, the callback is called.
//
// Unique values sent to the channel are stored in an internal map and all
// are processed once  the interval is up.
func debounce(interval time.Duration, input chan string, exit chan struct{}, firstCallback func(arg string), callback func(arg string)) {
	var items = make(map[string]bool)
	var item string
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case item = <-input:
			if _, ok := items[item]; !ok {
				firstCallback(item)
			}
			items[item] = true
			timer.Reset(interval)
		case <-timer.C:
			for path := range items {
				callback(path)
				delete(items, path)
			}
		case _, ok := <-exit:
			if !ok {
				return
			}
		}
	}
}

// fsTypeMap maps filesystem type identifiers to human-readable names.
var fsTypeMap = map[int64]string{
	0xEF53:     "ext2/ext3/ext4",
	0x58465342: "xfs",
	0x9123683E: "btrfs",
	0x6969:     "nfs",
	0xFF534D42: "cifs",
	0x65735546: "fuse",
}

// detectFSType detects the filesystem type of the given path using
func detectFSType(path string) string {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		log.Warnf("Failed to detect filesystem type: %v", err)
		return "unknown"
	}
	if name, ok := fsTypeMap[int64(stat.Type)]; ok {
		return name
	}
	return "unknown"
}

// isNetworkFS checks if the given filesystem type is a network filesystem.
func isNetworkFS(fs string) bool {
	return fs == "nfs" || fs == "cifs" || fs == "fuse" || fs == "unknown"
}

// FSWatcher provides a filesystem watcher to detect when mbtiles files are
// created, updated, or removed on the filesystem.
type FSWatcher struct {
	watcher    *fsnotify.Watcher
	svcSet     *handlers.ServiceSet
	generateID handlers.IDGenerator
}

// NewFSWatcher creates a new FSWatcher to watch the filesystem for changes to
// mbtiles files and updates the ServiceSet accordingly.
//
// The generateID function needs to be of the same type used when the tilesets
// were originally added to the ServiceSet.
func NewFSWatcher(svcSet *handlers.ServiceSet, generateID handlers.IDGenerator) (*FSWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &FSWatcher{
		watcher:    watcher,
		svcSet:     svcSet,
		generateID: generateID,
	}, nil
}

// Close closes the FSWatcher and stops watching the filesystem.
func (w *FSWatcher) Close() {
	if w.watcher != nil {
		w.watcher.Close()
	}
}

// handleFileChange processes a file change event for  mbtiles file.
func (w *FSWatcher) handleFileChange(path, baseDir string) {
	// Skip files still being written (SQLite -journal or -wal)
	if _, err := os.Stat(path + "-journal"); err == nil {
		log.Debugf("Tileset %q is currently being written", path)
		return
	}
	// Skip files still being written (SQLite -wal)
	if _, err := os.Stat(path + "-wal"); err == nil {
		log.Debugf("Tileset %q is currently being written (wal)", path)
		return
	}

	db, err := mbtiles.Open(path)
	if err != nil {
		return
	}
	db.Close()
	// Valid mbtiles file
	id, err := w.generateID(path, baseDir)
	if err != nil {
		log.Errorf("Failed to generate ID: %v", err)
		return
	}

	if w.svcSet.HasTileset(id) {
		w.svcSet.LockTileset(id)
		defer w.svcSet.UnlockTileset(id)
		if err := w.svcSet.UpdateTileset(id); err != nil {
			log.Errorf("Failed to update tileset: %v", err)
		} else {
			log.Infof("Updated tileset: %s", id)
		}
		return
	}

	w.svcSet.LockTileset(id)
	defer w.svcSet.UnlockTileset(id)
	if err := w.svcSet.AddTileset(path, id); err != nil {
		log.Errorf("Failed to add tileset: %v", err)
	} else {
		log.Infof("Added tileset: %s", id)
	}
}

// handleFileRemove processes a file removal event for  mbtiles file.
func (w *FSWatcher) handleFileRemove(path, baseDir string) {
	id, err := w.generateID(path, baseDir)
	if err != nil {
		log.Errorf("Failed to generate ID: %v", err)
		return
	}
	if w.svcSet.HasTileset(id) {
		if err := w.svcSet.RemoveTileset(id); err != nil {
			log.Errorf("Failed to remove tileset: %v", err)
		} else {
			log.Infof("Removed tileset: %s", id)
		}
	}
}

// polling directories for changes to mbtiles files
func (w *FSWatcher) startPolling(baseDir string, interval time.Duration) {
	log.Infof("Using polling mode to watch %s every %v", baseDir, interval)
	// fileState holds the modification time and size of a file
	type fileState struct {
		ModTime time.Time
		Size    int64
	}

	dirCache := make(map[string]map[string]fileState)
	mu := sync.Mutex{}
	// scanDir scans the directory for mbtiles files and returns a map of their states
	scanDir := func(path string) map[string]fileState {
		files := make(map[string]fileState)
		filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if filepath.Ext(p) != ".mbtiles" {
				return nil
			}
			files[p] = fileState{ModTime: info.ModTime(), Size: info.Size()}
			return nil
		})
		return files
	}

	// Initial scan
	mu.Lock()
	dirCache[baseDir] = scanDir(baseDir)
	for p := range dirCache[baseDir] {
		w.handleFileChange(p, baseDir)
	}
	mu.Unlock()
	// Start polling ticker
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		mu.Lock()
		current := scanDir(baseDir)

		// New or modified files
		for p, st := range current {
			oldSt, exists := dirCache[baseDir][p]
			if !exists || oldSt.ModTime != st.ModTime || oldSt.Size != st.Size {
				w.handleFileChange(p, baseDir)
			}
		}

		// Deleted files
		for p := range dirCache[baseDir] {
			if _, exists := current[p]; !exists {
				w.handleFileRemove(p, baseDir)
			}
		}

		dirCache[baseDir] = current
		mu.Unlock()
	}
}

// WatchDir sets up the filesystem watcher for baseDir and all existing subdirectories
func (w *FSWatcher) WatchDir(baseDir string) error {
	fsType := detectFSType(baseDir)
	log.Infof("Detected filesystem type: %s", fsType)
	// Use polling mode for network filesystems
	if isNetworkFS(fsType) {
		// Network disk → polling mode
		go w.startPolling(baseDir, 5*time.Second)
		return nil
	}

	// Local disk → fsnotify mode
	c := make(chan string, 1024) // buffered to prevent blocking
	exit := make(chan struct{})
	// Ensure the watcher is closed when done
	go debounce(500*time.Millisecond, c, exit,
		func(path string) {
			id, err := w.generateID(path, baseDir)
			if err != nil {
				log.Errorf("Failed to generate ID: %v", err)
				return
			}
			w.svcSet.LockTileset(id)
		},
		func(path string) {
			w.handleFileChange(path, baseDir)
		},
	)

	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}

				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && event.Op&fsnotify.Create != 0 {
					// dynamically watch new directories
					w.watcher.Add(event.Name)
				}

				ext := filepath.Ext(event.Name)
				if ext != ".mbtiles" {
					continue
				}

				// Skip files still being written
				if _, err := os.Stat(event.Name + "-journal"); err == nil {
					continue
				}
				if _, err := os.Stat(event.Name + "-wal"); err == nil {
					continue
				}

				if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
					select {
					case c <- event.Name:
					default:
						log.Warnf("Event channel full, skipping %s", event.Name)
					}
				} else if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					if _, err := os.Stat(event.Name); err == nil {
						select {
						case c <- event.Name:
						default:
							log.Warnf("Event channel full, skipping %s", event.Name)
						}
					} else {
						w.handleFileRemove(event.Name, baseDir)
					}
				}

			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Error(err)
			}
		}
	}()

	// Recursively add directories
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return w.watcher.Add(path)
		}
		return nil
	})
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
