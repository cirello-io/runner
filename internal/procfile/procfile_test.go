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

package procfile

import (
	"os"
	"strings"
	"testing"

	"cirello.io/runner/v3/internal/runner"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParse(t *testing.T) {
	const example = `workdir: $GOPATH/src/github.com/example/go-app
#this is a comment
observe: *.go *.js
ignore: /vendor
build-server: make server
web:  restart=onbuild waitfor=localhost:8888 ./server serve
web2: restart=fail waitfor=localhost:8888 ./server serve
web3: restart=fail waitfor=localhost:8888 ./server serve
formation: web:1 web2:2 web3:1
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
			Name: "build-server",
			Cmd:  "make server",

			WaitFor: "",
		},
		{
			Name: "web",
			Cmd:  "./server serve",

			WaitFor: "localhost:8888",
			Restart: runner.OnBuild,
		},
		{
			Name: "web2",
			Cmd:  "./server serve",

			WaitFor: "localhost:8888",
			Restart: runner.OnFailure,
		},
		{
			Name: "web3",
			Cmd:  "./server serve",

			WaitFor: "localhost:8888",
			Restart: runner.OnFailure,
		},
	}
	expected.Formation = map[string]int{
		"web":  1,
		"web2": 2,
		"web3": 1,
	}
	if !cmp.Equal(got, expected, cmpopts.IgnoreUnexported(runner.Runner{})) {
		t.Errorf("parser did not get the right result. \n%v", cmp.Diff(got, expected, cmpopts.IgnoreUnexported(runner.Runner{})))
	}
}

func TestParseErrors(t *testing.T) {
	t.Run("web=a", func(t *testing.T) {
		example := `formation: web:a`
		got, err := Parse(strings.NewReader(example))
		if err != nil {
			t.Error("unexpected error", err)
		}
		if q := got.Formation["web"]; q != 1 {
			t.Error("non-numeric process type formations should default to 1, got:", q)
		}
	})
	t.Run("web", func(t *testing.T) {
		example := `formation: web`
		got, err := Parse(strings.NewReader(example))
		if err != nil {
			t.Error("unexpected error", err)
		}
		if q := got.Formation["web"]; q != 1 {
			t.Error("non specified process type quantities should default to 1, got:", q)
		}
	})
	t.Run("empty", func(t *testing.T) {
		example := `formation:     `
		got, err := Parse(strings.NewReader(example))
		if err != nil {
			t.Error("unexpected error", err)
		}
		if l := len(got.Formation); l != 0 {
			t.Error("empty formation lines should result in empty formations maps, got:", l)
		}
	})
}
