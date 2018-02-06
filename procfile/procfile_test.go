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
web: group=service restart=always waitfor=localhost:8888 ./server serve
web2: sticky=1 group=service restart=fail waitfor=localhost:8888 ./server serve
formation: web=1 web2=2
malformed-line`

	got, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}

	expected := runner.New()
	expected.WorkDir = os.ExpandEnv("$GOPATH/src/github.com/example/go-app")
	expected.Observables = []string{"*.go", "*.js"}
	expected.SkipDirs = []string{"/vendor"}
	expected.Processes = []*runner.ProcessType{
		{
			Name:       "build-server",
			Cmd:        []string{"make server"},
			WaitBefore: "",
			WaitFor:    "",
		},
		{
			Name: "web",
			Cmd: []string{
				"./server serve",
			},
			WaitBefore: "",
			WaitFor:    "localhost:8888",
			Restart:    runner.Always,
			Group:      "service",
		},
		{
			Name: "web2",
			Cmd: []string{
				"./server serve",
			},
			WaitBefore: "",
			WaitFor:    "localhost:8888",
			Restart:    runner.OnFailure,
			Group:      "service",
			Sticky:     true,
		},
	}
	expected.Formation = map[string]int{
		"web":  1,
		"web2": 2,
	}

	if !reflect.DeepEqual(&got, &expected) {
		t.Errorf("parser did not get the right result. got: %#v\nexpected:%#v", &got, &expected)
	}
}

func TestParseErrors(t *testing.T) {
	example := `formation: web=a`
	got, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}
	if q := got.Formation["web"]; q != 1 {
		t.Error("non-numeric process type formations should default to 1, got:", q)
	}

	example = `formation: web`
	got, err = Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}
	if q := got.Formation["web"]; q != 1 {
		t.Error("non specified process type quantities should default to 1, got:", q)
	}

	example = `formation:     `
	got, err = Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}
	if l := len(got.Formation); l != 0 {
		t.Error("empty formation lines should result in empty formations maps, got:", l)
	}
}
