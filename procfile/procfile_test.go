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

package procfile

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"cirello.io/runner/runner"
)

func TestParse(t *testing.T) {
	const example = `workdir: $GOPATH/src/github.com/example/go-app

#this is a comment
observe: *.go *.js
ignore: /vendor
build-server: make server
web: restart=always waitfor=localhost:8888 ./server serve
web2: restart=fail waitfor=localhost:8888 ./server serve
malformed-line`

	got, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}

	expected := runner.Runner{
		WorkDir:     os.ExpandEnv("$GOPATH/src/github.com/example/go-app"),
		Observables: []string{"*.go", "*.js"},
		SkipDirs:    []string{"/vendor"},
		Processes: []*runner.ProcessType{
			&runner.ProcessType{
				Name:       "build-server",
				Cmd:        []string{"make server"},
				WaitBefore: "",
				WaitFor:    "",
			},
			&runner.ProcessType{
				Name: "web",
				Cmd: []string{
					"./server serve",
				},
				WaitBefore: "",
				WaitFor:    "localhost:8888",
				Restart:    runner.Always,
			},
			&runner.ProcessType{
				Name: "web2",
				Cmd: []string{
					"./server serve",
				},
				WaitBefore: "",
				WaitFor:    "localhost:8888",
				Restart:    runner.OnFailure,
			},
		},
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("parser did not get the right result. got: %#v\nexpected:%#v", got, expected)
	}
}
