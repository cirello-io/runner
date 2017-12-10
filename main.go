/*
Command runner is a very ugly and simple structured command executer that
monitor file changes to trigger service restarts.

Create a file name runner.json in the root of the project you want to run.

runner.json:

	{
		"workdir":"some-dir",
		"observables":[ "*.go" ],
		"skipdirs":[ "/vendor" ],
		"services":[
			{
				"name": "build",
				"cmd": [
					"make all"
				]
			},
			{
				"name": "name",
				"cmd": [
					"go build",
					"./serve"
				]
			},
			{
				"name": "client",
				"waitFor":"localhost:8888",
				"cmd": [
					"make cli",
					"./client"
				]
			}
		]
	}


Points of note: workdir follow the same rules for exec.Command.Dir, observables
uses filepath.Match on top of filepath.Base of full paths; skipDirs are relative
to workdir; each command in cmd are executed in isolated shells, they share no
state with each other.

Services name "build" will always be executed first and in order of declaration.
*/
package main // import "cirello.io/runner"

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"

	"cirello.io/runner/runner"
)

// DefaultProcfile is the file that runner will open by default if no custom
// is given.
const DefaultProcfile = "runner.json"

var (
	procfile = flag.String("procfile", DefaultProcfile, "procfile that should be read to start the application")
)

func init() {
	flag.Parse()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("runner: ")

	fn := DefaultProcfile
	if *procfile != "" {
		fn = *procfile
	}

	fd, err := os.Open(fn)
	if err != nil {
		log.Fatalln(err)
	}

	var s runner.Runner
	if err := json.NewDecoder(fd).Decode(&s); err != nil {
		log.Fatalln("cannot parse spec file:", err)
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
