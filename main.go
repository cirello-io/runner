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

/*
Command runner is a very ugly and simple structured command executer that
monitor file changes to trigger process restarts.

Create a file name Procfile in the root of the project you want to run, and add
the following content:

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web-a: group=web restart=always waitfor=localhost:8888 ./server serve alpha
	web-b: group=web restart=always waitfor=localhost:8888 ./server serve bravo
	db: restart=failure waitfor=localhost:8888 ./server db

Special process types:

- workdir: the working directory. Environment variables are expanded. It follows
the same rules for exec.Command.Dir.

- observe: a space separated list of file patterns to scan for. It uses
filepath.Match internally.

- ignore: a space separated list of ignored directories relative to workdir,
typically vendor directories.

- build*: process type name prefixed by "build" are always executed first and in
order of declaration. On failure, they halt the initialization.

- waitfor (in process type): target hostname and port that the runner will probe
before starting the process type.

- restart (in process type): "always" will restart the process type at every
build; "fail" will restart the process type on failure; "temporary" will start
the service once and not restart it on rebuilds; "loop" will restart the process
when it naturally terminates.

- group (in process type): group of processes that depend on each other. If a
process type fails, it will halt all others in the same group. If the
"restart" paramater is not set to "always" or "fail", the affected process
types will halt and not restart.

- sticky (in build process types): a sticky build is not interrupted when file
changes are detected.

- optional (in process types): does not start this process unless explicit told
so. The process type must be part of a group.
*/
package main // import "cirello.io/runner"

import (
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(0)
	facet := filepath.Base(os.Args[0])
	switch facet {
	case "waitfor":
		log.SetPrefix("waitfor: ")
		waitfor()
	default:
		log.SetPrefix("runner: ")
		run()
	}

}
