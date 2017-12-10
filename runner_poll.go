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

// +build poll

package runner

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s Runner) monitorWorkDir() (<-chan struct{}, error) {
	lastHash, err := s.calculateObservablesHash()
	if err != nil {
		return nil, fmt.Errorf("can't calculate work dir hash: %v", err)
	}
	ch := make(chan struct{})

	go func() {
		tries := 0
		for range time.Tick(100 * time.Millisecond) {
			currentHash, err := s.calculateObservablesHash()
			if err != nil {
				log.Println("can't calculate work dir hash on tick:", err)
				continue
			}
			if lastHash != currentHash {
				lastHash = currentHash
				ch <- struct{}{}
				tries = 0
				continue
			}
			if tries < 5 {
				tries++
			}
			time.Sleep(time.Duration(tries) * time.Second)
		}
	}()

	return ch, nil
}

func (s Runner) calculateObservablesHash() (string, error) {
	hash := sha1.New()
	err := filepath.Walk(s.WorkDir, func(path string, info os.FileInfo, err error) error {
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
				fmt.Fprintln(hash, p, path, info.Name(), info.Size(), info.ModTime())
			}
		}
		return nil
	})
	return fmt.Sprintf("%x", hash.Sum(nil)), err
}
