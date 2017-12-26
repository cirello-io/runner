# Runner

[![GoDoc](https://godoc.org/cirello.io/runner/runner?status.svg)](https://godoc.org/cirello.io/runner/runner)
[![Go Report Card](https://goreportcard.com/badge/cirello.io/runner)](https://goreportcard.com/report/cirello.io/runner)
[![License](https://img.shields.io/badge/license-apache%202.0-blue.svg)](https://choosealicense.com/licenses/apache-2.0/)

runner is a structured command executer that monitor file changes to trigger
process restarts.

Create a file name Procfile in the root of the project you want to run, and add
the following content:

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web: restart=fail waitfor=localhost:8888 ./server serve
	web-a: group=web restart=always waitfor=localhost:8888 ./server serve alpha
	web-b: group=web restart=always waitfor=localhost:8888 ./server serve bravo
	db: restart=failure waitfor=web ./server db

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

- group (in process type): group of processes that depend on each other. If a
process type fails, it will halt all others in the same group. If the
"restart" paramater is not set to "always" or "fail", the affected process
types will halt and not restart.

## CLI parameters

```Shell
runner - simple Procfile runner

usage: runner [-convert] [Procfile]

Options:
  -convert
    	takes a declared Procfile and prints as JSON to standard output
  -env file
    	environment file to be loaded for all processes. (default ".env")
  -formation procTypeA=# procTypeB=# ... procTypeN=#
    	formation allows to start more than one instance of a process type, format: procTypeA=# procTypeB=# ... procTypeN=#
  -port PORT
    	base IP port used to set $PORT for each process type (default 5000)
  -skip procTypeA procTypeB procTypeN
    	does not run some of the process types, format: procTypeA procTypeB procTypeN
```

`-convert` allows you to generate a JSON version of the Procfile. This format
is more verbose but allows for more options. It can be used to add more steps
for each process type and to network readiness test before the first step, or
before the last one. [Refer to this datastructure to understand its possibilities.](https://godoc.org/cirello.io/runner/runner#Runner)

`-env file` loads the environment file common to all process types. It must be
in the format below:
```
VARIABLENAME=VALUE
VARIABLENAME=VALUE
```
Note: one environment variable per line. If the environment file is set, the
shell environment is discarded.

`-formation procTypeA=# procTypeB=# ... procTypeN=#` can be used to start more
than one instance of a process type. It is commonly used to start many
supporting background workers to an application.

`-port PORT` is the base IP port number used for each process type. It passes
the port number as an environment variable named `$PORT` to the process, and
it can be used as means to facilitate the application start up.

`-skip procTypeA procTypeB procTypeN` allows for partial execution of a Procfile.
If a formation is given, it does not start any instance of the specified process
type.

## Environment variables available to processes

Each process will have 3 environment variables available.

`PS` is the name which the runner has christened the process.

`PORT` is the IP port which the runner has indicated to that instance of a
service to bind itself to.

`DISCOVERY` is the HTTP service that returns a JSON describing each process
type port. This assumes the process has honored the `PORT` variable and bound
itself to the configured one.

## Installation
`go get [-f -u] [-tags poll] cirello.io/runner`

Use `-tags poll` if the fsnotify fails.

http://godoc.org/cirello.io/runner
