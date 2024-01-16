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

//go:build !fswatch
// +build !fswatch

package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Runner) monitorWorkDir(ctx context.Context) (<-chan string, error) {
	if _, err := os.Stat(s.WorkDir); err != nil {
		return nil, err
	}
	triggereds := make(chan string, 1024)
	memo := make(map[string]time.Time)
	go func() {
		for {
			_ = filepath.Walk(s.WorkDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					for _, skipDir := range s.SkipDirs {
						if skipDir == "" {
							continue
						}
						if strings.HasPrefix(path, filepath.Join(s.WorkDir, skipDir)) {
							return filepath.SkipDir
						}
					}
					return nil
				}
				for _, p := range s.Observables {
					if match(p, path) {
						mtime := info.ModTime()
						memoMTime, ok := memo[path]
						if !ok {
							memo[path] = mtime
							memoMTime = mtime
						}
						if !mtime.Equal(memoMTime) {
							memo[path] = mtime
							triggereds <- path
							break
						}
					}
				}
				return nil
			})
		}
	}()
	go func() { triggereds <- "" }()
	return triggereds, nil
}
