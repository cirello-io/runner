# Runner

[![GoDoc](https://godoc.org/cirello.io/runner/runner?status.svg)](https://godoc.org/cirello.io/runner/runner)
[![Go Report Card](https://goreportcard.com/badge/cirello.io/runner)](https://goreportcard.com/report/cirello.io/runner)
[![License](https://img.shields.io/badge/license-apache%202.0-blue.svg)](https://choosealicense.com/licenses/apache-2.0/)

runner is a very ugly and simple structured command executer that
monitor file changes to trigger process restarts.

Create a file name Procfile in the root of the project you want to run, and add
the following content:

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web: restart=fail waitfor=localhost:8888 ./server serve

Special process type names:

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

- restart (in process type): "always" will restart the process type every time;
"fail" will restart the process type on failure.

## Installation
`go get [-f -u] [-tags poll] cirello.io/runner`

Use `-tags poll` if the fsnotify fails.

http://godoc.org/cirello.io/runner
