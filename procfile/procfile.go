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
// Special service names:
//
// - workdir: the working directory, and environment variables are expanded.
//
// - observe: a space separated list of file patterns to scan for. It uses filepath.Match internally.
//
// - ignore: a space separated list of directories to ignore, typically vendor directories.
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
		procType, command := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch strings.ToLower(procType) {
		case "":
			continue
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
