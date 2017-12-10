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
