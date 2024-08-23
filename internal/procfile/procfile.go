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

// Package procfile provides a parser that know how read an extended version
// of Procfile as described by Heroku (https://devcenter.heroku.com/articles/procfile).
//
// This version allows to set specific behaviors per process type.
//
// Example:
//
//	workdir: $GOPATH/src/github.com/example/go-app
//	basePort: 5000
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
// - baseport: when set to a number, it will be used as the starting point for
// the $PORT environment variable. Each process type will have its own exclusive
// $PORT variable value.
//
// - observe: a space separated list of file patterns to scan for. It uses
// filepath.Match internally. File patterns preceded with exclamation mark (!)
// will not trigger builds.
//
// - ignore: a space separated list of ignored directories relative to workdir,
// typically vendor directories.
//
// - formation: allows to control how many instances of a process type are
// started, format: procTypeA:# procTypeB:# ... procTypeN:#. If `procType` is
// absent, it is not started. Empty formations start one of each process.
//
// - waitfor (in process type): target hostname and port that the runner will
// probe before starting the process type.
//
// - restart (in process type): "onbuild" will restart the process type at every
// build; "fail" will restart the process type on failure; "loop" restart the
// process when it naturally terminates; "temporary" runs the process only once.
//
// - signal (in process types): "SIGTERM", "term", or "15" terminates the
// process; "SIGKILL", "kill", or "9" kills the process. The default is
// "SIGKILL".
//
// - timeout (in process types): duration (in Go format) to wait after
// sending the signal to the process.
package procfile

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"cirello.io/runner/v2/internal/runner"
)

// ParseFormation interprets a string in the format "proc=quantity
// proc2=quantity"
func ParseFormation(s string) map[string]int {
	procs := strings.Split(s, " ")
	ret := make(map[string]int, len(procs))
	for _, proc := range procs {
		procName, count, _ := strings.Cut(proc, ":")
		procName = strings.TrimSpace(procName)
		if procName == "" {
			continue
		}
		ret[procName] = 1
		if quantity, err := strconv.Atoi(strings.TrimSpace(count)); err == nil {
			ret[procName] = quantity
		}
	}
	return ret
}

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
		case "baseport":
			port, err := strconv.Atoi(command)
			if err != nil {
				return rnr, err
			}
			if port < 1 || port > 65535 {
				return rnr, errors.New("invalid IP port")
			}
			rnr.BasePort = port
		case "observe", "watch":
			rnr.Observables = strings.Split(command, " ")
		case "ignore":
			rnr.SkipDirs = strings.Split(command, " ")
		case "formation":
			rnr.Formation = ParseFormation(command)
		default:
			proc := runner.ProcessType{Name: procType}
			parts := strings.Split(command, " ")
			var command []string
			for _, part := range parts {
				if strings.HasPrefix(part, "waitfor=") {
					proc.WaitFor = strings.TrimPrefix(part, "waitfor=")
					continue
				}
				if strings.HasPrefix(part, "restart=") {
					restartMode := strings.TrimPrefix(part, "restart=")
					proc.Restart = runner.ParseRestartMode(restartMode)
					continue
				}
				if strings.HasPrefix(part, "signal=") {
					proc.Signal = runner.ParseSignal(strings.TrimPrefix(part, "signal="))
					continue
				}
				if strings.HasPrefix(part, "timeout=") {
					timeout, err := time.ParseDuration(strings.TrimPrefix(part, "timeout="))
					if err != nil {
						return rnr, err
					}
					proc.Timeout = timeout
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
