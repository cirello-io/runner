# Runner

[![GoDoc](https://godoc.org/cirello.io/runner/runner?status.svg)](https://godoc.org/cirello.io/runner/runner)
[![Go Report Card](https://goreportcard.com/badge/cirello.io/runner)](https://goreportcard.com/report/cirello.io/runner)
[![License](https://img.shields.io/badge/license-apache%202.0-blue.svg)](https://choosealicense.com/licenses/apache-2.0/)

runner is a very ugly and simple structured command executer that
monitor file changes to trigger service restarts.

Create a file name Procfile in the root of the project you want to run.

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web: waitfor=localhost:8888 ./server serve

On each process type, you can declare "waitfor=hostname:port" to check for the
readiness of a dependency through a network check.

Special service names:

- workdir: the working directory. Environment variables are expanded. It follow
he same rules for exec.Command.Dir.

- observe: a space separated list of file patterns to scan for. It uses
filepath.Match internally.

- ignore: a space separated list of ignored directories relative to workdir,
typically vendor directories.

- build*: process type name prefixed by "build" are always executed first and in
  order of declaration. On failure, they halt the initialization.

## Installation
go get [-u] [-tags fswatch] cirello.io/runner

http://godoc.org/cirello.io/runner
