package main

import (
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"

	mbtiles "github.com/brendan-ward/mbtiles-go"
	"github.com/consbio/mbtileserver/handlers"
	"github.com/fsnotify/fsnotify"
)

// debounce debounces requests to a callback function to occur no more
// frequently than interval; once this is reached, the callback is called.
//
// Unique values sent to the channel are stored in an internal map and all
// are processed once the the interval is up.
func debounce(interval time.Duration, input chan string, exit chan struct{}, firstCallback func(arg string), callback func(arg string)) {
	// keep a log of unique paths
	var items = make(map[string]bool)
	var item string
	timer := time.NewTimer(interval)
	for {
		select {
		case item = <-input:
			if _, ok := items[item]; !ok {
				// first time we see a given path, we need to call lockHandler
				// to lock it (unlocked by callback)
				firstCallback(item)
			}
			items[item] = true
			timer.Reset(interval)
		case <-timer.C:
			for path := range items {
				callback(path)
				delete(items, path)
			}
		case <-exit:
			//log.Info("debounce return")
			return
		}
	}
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

// WatchDir sets up the filesystem watcher for baseDir and all existing subdirectories
func (w *FSWatcher) WatchDir(baseDir string) error {
	c := make(chan string)
	exit := make(chan struct{})
	// debounced call to create / update tileset
	go debounce(500*time.Millisecond, c, exit, func(path string) {
		// callback for first time path is debounced
		id, err := w.generateID(path, baseDir)
		if err != nil {
			log.Errorf("Could not create ID for tileset %q\n%v", path, err)
			return
		}
		// lock tileset for writing, if it exists
		w.svcSet.LockTileset(id)
	}, func(path string) {
		// callback after debouncing incoming requests

		// Verify that file can be opened with mbtiles-go, which runs
		// validation on open.
		// If file cannot be opened, assume it is still being written / copied.
		db, err := mbtiles.Open(path)
		if err != nil {
			return
		}
		db.Close()

		// determine file ID for tileset
		id, err := w.generateID(path, baseDir)
		if err != nil {
			log.Errorf("Could not create ID for tileset %q\n%v", path, err)
			return
		}

		// update existing tileset
		if w.svcSet.HasTileset(id) {
			err = w.svcSet.UpdateTileset(id)
			if err != nil {
				log.Errorf("Could not update tileset %q with ID %q\n%v", path, id, err)
			} else {
				// only unlock if successfully updated
				w.svcSet.UnlockTileset(id)
				log.Infof("Updated tileset %q with ID %q\n", path, id)
			}
			return
		}

		// create new tileset
		err = w.svcSet.AddTileset(path, id)
		if err != nil {
			log.Errorf("Could not add tileset for %q with ID %q\n%v", path, id, err)
		} else {
			log.Infof("Updated tileset %q with ID %q\n", path, id)
		}
		return
	})
	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					log.Errorf("error in filewatcher for %q, exiting filewatcher", event.Name)
					return
				}

				if !((event.Op&fsnotify.Create == fsnotify.Create) ||
					(event.Op&fsnotify.Write == fsnotify.Write) ||
					(event.Op&fsnotify.Remove == fsnotify.Remove) ||
					(event.Op&fsnotify.Rename == fsnotify.Rename)) {
					continue
				}

				path := event.Name
				if ext := filepath.Ext(path); ext != ".mbtiles" {
					if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write ||
						event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
						exit <- struct{}{}
						err := w.WatchDir(baseDir)
						if err != nil {
							return
						}
						log.Info("reload watch dir")
						return
					} else {
						continue
					}

				}

				if _, err := os.Stat(path + "-journal"); err == nil {
					// Don't try to load .mbtiles files that are being written
					log.Debugf("Tileset %q is currently being created or is incomplete\n", path)
					continue
				}

				if (event.Op&fsnotify.Create == fsnotify.Create) ||
					(event.Op&fsnotify.Write == fsnotify.Write) {
					// This event may get called multiple times while a file is being copied into a watched directory,
					// so we debounce this instead.
					c <- path
					continue
				}

				if (event.Op&fsnotify.Remove == fsnotify.Remove) || (event.Op&fsnotify.Rename == fsnotify.Rename) {
					// some file move events trigger remove / rename, so if the file still exists, assume it is
					// one of these
					_, err := os.Stat(path)
					if err == nil {
						// debounce to give it a little more time to update, if needed
						c <- path
						continue
					}

					// remove tileset immediately so that there are not other errors in request handlers
					id, err := w.generateID(path, baseDir)
					if err != nil {
						log.Errorf("Could not create ID for tileset %q\n%v", path, err)
					}
					err = w.svcSet.RemoveTileset(id)
					if err != nil {
						log.Errorf("Could not remove tileset %q with ID %q\n%v", path, id, err)
					} else {
						log.Infof("Removed tileset %q with ID %q\n", path, id)
					}
				}

			case err, ok := <-w.watcher.Errors:
				if !ok {
					log.Errorf("error in filewatcher, exiting filewatcher")
					return
				}
				log.Error(err)
			}
		}
	}()

	err := filepath.Walk(baseDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsDir() {
				return w.watcher.Add(path)
			}
			return nil
		})
	if err != nil {
		return err
	}

	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return false
	}
	return true
}
