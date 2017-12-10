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

// Package procfile provides a parser that know how read an extended version
// of Procfile as described by Heroku (https://devcenter.heroku.com/articles/procfile).
//
// This version allows to set specific behaviors per process type.
//
// Example:
//
//	workdir: $GOPATH/src/github.com/example/go-app
//	observe: *.go *.js
//	ignore: /vendor
//	build-server: make server
//	web: waitfor=localhost:8888 waitbefore=localhost:2122 ./server serve
//
// Special process type names:
//
// - workdir: the working directory. Environment variables are expanded. It
//   follow the same rules for exec.Command.Dir.
//
// - observe: a space separated list of file patterns to scan for. It uses
//   filepath.Match internally.
//
// - ignore: a space separated list of ignored directories relative to workdir,
//   typically vendor directories.
package procfile // import "cirello.io/runner/procfile"

import (
	"bufio"
	"io"
	"os"
	"strings"

	"cirello.io/runner/runner"
)

// Parse takes a reader that contains an extended Procfile.
func Parse(r io.Reader) (runner.Runner, error) {
	rnr := runner.Runner{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		procType := strings.TrimSpace(parts[0])
		if procType == "" {
			continue
		}
		command := strings.TrimSpace(parts[1])
		switch strings.ToLower(procType) {
		case "workdir":
			rnr.WorkDir = os.ExpandEnv(command)
		case "observe":
			rnr.Observables = strings.Split(command, " ")
		case "ignore":
			rnr.SkipDirs = strings.Split(command, " ")
		default:
			svc := runner.Service{Name: procType}
			parts := strings.Split(command, " ")
			var command []string
			for _, part := range parts {
				if strings.HasPrefix(part, "waitfor=") {
					svc.WaitFor = strings.TrimPrefix(part, "waitfor=")
					continue
				}
				command = append(command, part)
			}
			svc.Cmd = []string{strings.TrimSpace(strings.Join(command, " "))}
			rnr.Services = append(rnr.Services, &svc)
		}
	}

	return rnr, scanner.Err()
}