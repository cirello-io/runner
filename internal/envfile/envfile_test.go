// Copyright 2024 github.com/ucirello, cirello.io, U. Cirello
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

package envfile

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

func TestParse(t *testing.T) {
	err := filepath.Walk("_testdata", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".txtar" {
			return nil
		}
		archive, err := txtar.ParseFile(path)
		if err != nil {
			return err
		}
		var envFile []byte
		var expected string
		for _, f := range archive.Files {
			switch f.Name {
			case ".env":
				envFile = f.Data
			case "expected":
				expected = string(f.Data)
			}
		}
		t.Run(path, func(t *testing.T) {
			v, err := Parse(bytes.NewReader(envFile))
			if err != nil {
				t.Fatalf("cannot parse env file (%q): %v", path, err)
			}
			if strings.TrimSpace(fmt.Sprint(v)) != strings.TrimSpace(expected) {
				t.Log("got:", strings.TrimSpace(fmt.Sprint(v)))
				t.Log("expected:", strings.TrimSpace(expected))
				t.Error("environment file parsing is broken")
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
