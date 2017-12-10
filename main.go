/*
Command runner is a very ugly and simple structured command executer that
monitor file changes to trigger service restarts.

Create a file name Procfile in the root of the project you want to run.

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web: waitfor=localhost:8888 ./server serve

Points of note: workdir follow the same rules for exec.Command.Dir, observe
uses filepath.Match on top of filepath.Base of full paths; ignore are relative
to workdir.

In the service, you can declare "waitfor=hostname:port" to check for the
readiness of a dependency through network check.

Services whose names are prefixed by "build" will always be executed first and
in order of declaration.

Special service names:

- workdir: the working directory, and environment variables are expanded.

- observe: a space separated list of file patterns to scan for. It uses filepath.Match internally.

- ignore: a space separated list of directories to ignore, typically vendor directories.
*/
package main // import "cirello.io/runner"

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"cirello.io/runner/procfile"
	"cirello.io/runner/runner"
)

// DefaultProcfile is the file that runner will open by default if no custom
// is given.
const DefaultProcfile = "Procfile"

var (
	procfileFn = flag.String("procfile", DefaultProcfile, "procfile that should be read to start the application")
)

func init() {
	flag.Parse()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("runner: ")

	fn := DefaultProcfile
	if *procfileFn != "" {
		fn = *procfileFn
	}

	fd, err := os.Open(fn)
	if err != nil {
		log.Fatalln(err)
	}

	var s runner.Runner
	switch filepath.Ext(fn) {
	case ".json":
		if err := json.NewDecoder(fd).Decode(&s); err != nil {
			log.Fatalln("cannot parse spec file (json):", err)
		}
	default:
		s, err = procfile.Parse(fd)
		if err != nil {
			log.Fatalln("cannot parse spec file (procfile):", err)
		}
	}

	s.WorkDir = os.ExpandEnv(s.WorkDir)
	if s.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalln("cannot load current workdir", err)
		}
		s.WorkDir = wd
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		log.Println("shutting down")
		os.Exit(0)
	}()

	if err := s.Start(); err != nil {
		log.Fatalln("cannot serve:", err)
	}
}
