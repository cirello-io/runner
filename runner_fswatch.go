// Copyright 2017 github.com/ucirello
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !poll

package runner

import (
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
