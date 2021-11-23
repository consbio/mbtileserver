package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/consbio/mbtileserver/handlers"
	"github.com/fsnotify/fsnotify"
)

type FSWatcher struct {
	watcher    *fsnotify.Watcher
	svcSet     *handlers.ServiceSet
	generateID handlers.IDGenerator
}

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

func (w *FSWatcher) Close() {
	fmt.Println("Close called")
	if w.watcher != nil {
		w.watcher.Close()
	}
}

func (w *FSWatcher) WatchDir(baseDir string) error {
	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					log.Errorf("error in filewatcher for %q, exiting filewatcher", event.Name)
					return
				}

				if !strings.Contains(filepath.Ext(event.Name), "mbtiles") {
					continue
				}

				if (event.Op == fsnotify.Write) || (event.Op == fsnotify.Create) {
					// warning: this event may get called multiple times
					log.Infoln("created / modified tileset:", event.Name)
					id, err := w.generateID(event.Name, baseDir)
					if err != nil {
						log.Errorf("Could not create ID for tileset %q\n%v", event.Name, err)
					}

					// WARNING: this will fail on initial event if file is not yet in place and complete
					if w.svcSet.HasTileset(id) {
						err = w.svcSet.UpdateTileset(id)
						if err != nil {
							log.Errorf("Could not update tileset for %q with ID %q\n%v", event.Name, id, err)
						} else {
							log.Infof("Updated tilesets for %q with ID %q\n", event.Name, id)
						}
					} else {
						err = w.svcSet.AddTileset(event.Name, id)
						if err != nil {
							log.Errorf("Could not add tileset for %q with ID %q\n%v", event.Name, id, err)
						} else {
							log.Infof("Updated tilesets for %q with ID %q\n", event.Name, id)
						}
					}
				}

				if (event.Op == fsnotify.Remove) || (event.Op == fsnotify.Rename) {
					log.Infoln("removing tileset:", event.Name)
					id, err := w.generateID(event.Name, baseDir)
					if err != nil {
						log.Errorf("Could not determine ID for tileset %q\n%v", event.Name, err)
					}
					err = w.svcSet.RemoveTileset(id)
					if err != nil {
						log.Errorf("Could not remove tileset for %q with ID %q\n%v", event.Name, id, err)
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
			log.Infoln("Adding path", path)
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
