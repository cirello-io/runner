// +build fswatch

package runner

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func (s Runner) monitorWorkDir() (<-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ch := make(chan struct{})
	memo := make(map[string]struct{})

	err = filepath.Walk(s.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			for _, skipDir := range s.SkipDirs {
				if strings.HasPrefix(path, filepath.Join(s.WorkDir, skipDir)) {
					return filepath.SkipDir
				}
			}
		}
		for _, p := range s.Observables {
			if matched, err := filepath.Match(p, filepath.Base(path)); err == nil && matched {
				dir := filepath.Dir(path)
				if _, ok := memo[dir]; !ok {
					memo[dir] = struct{}{}
					watcher.Add(dir)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	log.Println("monitoring", len(memo), "directories")

	testFile := func(p, path string) {
		fmt.Println(p, path)
		matched, err := filepath.Match(p, filepath.Base(path))
		if err == nil && matched {
			ch <- struct{}{}
		}
	}
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write != fsnotify.Write {
					continue
				}
				for _, p := range s.Observables {
					testFile(p, event.Name)
				}
			case err := <-watcher.Errors:
				log.Println("fswatch error:", err)
			}
		}
	}()

	return ch, nil
}
