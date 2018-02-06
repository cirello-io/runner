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
//	formation: web=2
//	build-server: make server
//	web: restart=fail waitfor=localhost:8888 ./server serve
//
// Special process type names:
//
// - workdir: the working directory. Environment variables are expanded. It
// follows the same rules for exec.Command.Dir.
//
// - observe: a space separated list of file patterns to scan for. It uses
// filepath.Match internally.
//
// - ignore: a space separated list of ignored directories relative to workdir,
// typically vendor directories.
//
// - formation: allows to start more than one instance for a given process type.
// if the process type is declared with zero ("proc=0"), it is not started. Non
// declared process types are started once. Each process type has its own
// exclusive $PORT variable value.
//
// - waitfor (in process type): target hostname and port that the runner will
// probe before starting the process type.
//
// - restart (in process type): "always" will restart the process type every
// time; "fail" will restart the process type on failure.
//
// - group (in process type): group of processes that depend on each other. If a
// process type fails, it will halt all others in the same group. If the
// "restart" paramater is not set to "always" or "fail", the affected process
// types will halt and not restart.
//
// - sticky (in build process types): a sticky build is not interrupted when
// file changes are detected.
//
// Although internally runner.Runner supports waitbefore and multi-command
// processes, for simplicity of interface these features have been disabled in
// Procfile parser.
package procfile // import "cirello.io/runner/procfile"

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"

	"cirello.io/runner/runner"
)

// Parse takes a reader that contains an extended Procfile.
func Parse(r io.Reader) (runner.Runner, error) {
	rnr := runner.New()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// loosen translation of the official regex:
		// ^*([A-Za-z0-9_-]+):\s*(.+)$
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		procType, command := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch strings.ToLower(procType) {
		case "workdir":
			rnr.WorkDir = os.ExpandEnv(command)
		case "observe", "watch":
			rnr.Observables = strings.Split(command, " ")
		case "ignore":
			rnr.SkipDirs = strings.Split(command, " ")
		case "formation":
			procs := strings.Split(command, " ")
			for _, proc := range procs {
				parts := strings.Split(proc, "=")
				switch len(parts) {
				case 0:
					continue
				case 1:
					procName := strings.TrimSpace(parts[0])
					if procName == "" {
						continue
					}
					rnr.Formation[procName] = 1
					continue
				default:
					procName := strings.TrimSpace(parts[0])
					quantity, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err != nil {
						quantity = 1
					}
					rnr.Formation[procName] = quantity
				}
			}
		default:
			proc := runner.ProcessType{Name: procType}
			parts := strings.Split(command, " ")
			var command []string
			for _, part := range parts {
				if strings.HasPrefix(part, "waitfor=") {
					proc.WaitFor = strings.TrimPrefix(part, "waitfor=")
					continue
				}
				if strings.HasPrefix(part, "sticky=") {
					sticky, err := strconv.ParseBool(strings.TrimPrefix(part, "sticky="))
					if err != nil {
						return rnr, err
					}
					proc.Sticky = sticky
					continue
				}
				if strings.HasPrefix(part, "restart=") {
					restartMode := strings.TrimPrefix(part, "restart=")
					proc.Restart = runner.ParseRestartMode(restartMode)
					continue
				}
				if strings.HasPrefix(part, "group=") {
					proc.Group = strings.TrimPrefix(part, "group=")
					continue
				}
				command = append(command, part)
			}
			proc.Cmd = []string{strings.TrimSpace(strings.Join(command, " "))}
			rnr.Processes = append(rnr.Processes, &proc)
		}
	}

	return rnr, scanner.Err()
}
