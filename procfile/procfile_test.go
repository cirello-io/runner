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
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const example = `workdir: $GOPATH/src/github.com/example/go-app

observe: *.go *.js
ignore: /vendor
build-server: make server
web: waitfor=localhost:8888 waitbefore=localhost:2122 ./server serve`

	got, err := Parse(strings.NewReader(example))
	if err != nil {
		t.Error("unexpected error", err)
	}

	if l := len(got.Observables); l != 2 {
		t.Error("unexpected number of observables", l)
	}

	if l := len(got.SkipDirs); l != 1 {
		t.Error("unexpected number of ignored dirs", l)
	}

	if l := len(got.Services); l != 2 {
		t.Error("unexpected number of services", l)
	}

	gotSVC := got.Services[1]

	if gotSVC.WaitFor == "" {
		t.Error("service WaitFor is missing")
	}
	if l := len(gotSVC.Cmd); l != 1 {
		t.Error("unexpected number of commands", l)
	}
}
