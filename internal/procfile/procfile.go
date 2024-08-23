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
// filepath.Match internally. File patterns preceded with exclamation mark (!)
// will not trigger builds.
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
// - restart (in process type): "onbuild" will restart the process type at every
// build; "fail" will restart the process type on failure; "loop" restart the
// process when it naturally terminates; "temporary" runs the process only once.
//
// - group (in process type): group of processes that depend on each other. If a
// process type fails, it will halt all others in the same group. If the
// "restart" parameter is not set to "always" or "fail", the affected process
// types will halt and not restart.
//
// - signal (in process types): "SIGTERM", "term", or "15" terminates the
// process; "SIGKILL", "kill", or "9" kills the process. The default is
// "SIGKILL".
//
// - signalWait (in process types): duration to wait after sending the signal to
// the process.
//
// - sticky (in build process types): a sticky build is not interrupted when
// file changes are detected.
//
// - optional (in process types): does not start this process unless explicit
// told so.
package procfile

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"cirello.io/runner/v2/internal/runner"
)

// Parse takes a reader that contains an extended Procfile.
func Parse(r io.Reader) (*runner.Runner, error) {
	rnr := runner.New()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// loosen translation of the official regex:
		// ^*([A-Za-z0-9_-]+):\s*(.+)$
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		procType, command, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		procType, command = strings.TrimSpace(procType), strings.TrimSpace(command)
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
				procName, count, _ := strings.Cut(proc, "=")
				procName = strings.TrimSpace(procName)
				if procName == "" {
					continue
				}
				rnr.Formation[procName] = 1
				if quantity, err := strconv.Atoi(strings.TrimSpace(count)); err == nil {
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
				if strings.HasPrefix(part, "signal=") {
					proc.Signal = runner.ParseSignal(strings.TrimPrefix(part, "signal="))
					continue
				}
				if strings.HasPrefix(part, "signalWait=") {
					signalWait, err := time.ParseDuration(strings.TrimPrefix(part, "signalWait="))
					if err != nil {
						return rnr, err
					}
					proc.SignalWait = signalWait
					continue
				}
				if strings.HasPrefix(part, "optional=") {
					optional, err := strconv.ParseBool(strings.TrimPrefix(part, "optional="))
					if err != nil {
						return rnr, err
					}
					proc.Optional = optional
					continue
				}
				command = append(command, part)
			}
			proc.Cmd = strings.TrimSpace(strings.Join(command, " "))
			rnr.Processes = append(rnr.Processes, &proc)
		}
	}

	return rnr, scanner.Err()
}
