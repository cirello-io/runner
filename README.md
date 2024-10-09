# Runner

[![GoDoc](https://pkg.go.dev/badge/cirello.io/runner/v3)](https://pkg.go.dev/cirello.io/runner/v3)
[![License](https://img.shields.io/badge/license-apache%202.0-blue.svg)](https://choosealicense.com/licenses/apache-2.0/)

runner is a structured command executer for Unix systems that monitor file
changes to trigger process restarts.

Create a file name Procfile in the root of the project you want to run, and add
the following content:

	workdir: $GOPATH/src/github.com/example/go-app
	formation: web web-a web-b db optional-service=0
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web: restart=fail waitfor=localhost:8888 ./server serve
	web-a: restart=onbuild waitfor=localhost:8888 ./server serve alpha
	web-b: restart=onbuild waitfor=localhost:8888 ./server serve bravo
	db: restart=failure waitfor=web ./server db
	optional-service: ./optional-service

Special process type names:

- workdir: the working directory. Environment variables are expanded. It follows
the same rules for exec.Command.Dir.

- observe: a space separated list of file patterns to scan for. It uses
filepath.Match internally. File patterns preceded with exclamation mark (!) will
not trigger builds.

- ignore: a space separated list of ignored directories relative to workdir,
typically vendor directories.

- formation: allows to control how many instances of a process type are
started, format: procTypeA:# procTypeB:# ... procTypeN:#. If `procType` is
absent, it is not started. Empty formations start one of each process.

- build*: process type name prefixed by "build" are always executed first and in
order of declaration. On failure, they halt the initialization.

- waitfor (in process type): target hostname and port that the runner will probe
before starting the process type.

- restart (in process type): "onbuild" will restart the process type at every
build; "fail" will restart the process type on failure; "loop" restart the
process when it naturally terminates; "temporary" runs the process only once.

## CLI parameters

```Shell
NAME:
   runner - simple Procfile runner

USAGE:
   runner [global options] command [command options] [arguments...]

VERSION:
   v3 (881f4e039f4ea6342464d4dc6ee3df9d362f5712)

COMMANDS:
   logs     Follows logs from running processes
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --service-discovery value                            service discovery address (default: "localhost:64000")
   --formation procTypeA:# procTypeB:# ... procTypeN:#  formation allows to control how many instances of a process type are started, format: procTypeA:# procTypeB:# ... procTypeN:#. If `procType` is absent, it is not started. Empty formations start one of each process.
   --env file                                           environment file to be loaded for all processes, if the file is absent, then this parameter is ignored. (default: ".env")
   --skip procTypeA procTypeB procTypeN                 does not run some of the process types, format: procTypeA procTypeB procTypeN
   --only procTypeA procTypeB procTypeN                 only runs some of the process types, format: procTypeA procTypeB procTypeN
   --optional procTypeA procTypeB procTypeN             forcefully runs some of the process types, format: procTypeA procTypeB procTypeN
   --help, -h                                           show help
   --version, -v                                        print the version
```

`-env file` loads the environment file common to all process types. It must be
in the format below:
```
VARIABLENAME=VALUE
VARIABLENAME=VALUE
```
Note: one environment variable per line.

`--formation procTypeA:# procTypeB:# ... procTypeN:#` allows to control
how many instances of a process type are started, format: procTypeA:#
procTypeB:# ... procTypeN:#. If `procType` is absent, it is not started. Empty
formations start one of each process.


## Environment variables available to processes

Each process will have three environment variables available.

`PS` is the name which the runner has christened the process.

`DISCOVERY` is the HTTP service that returns a JSON describing each process
type port. This assumes the process has honored the `PORT` variable and bound
itself to the configured one.


## Support

`runner/v3` is only supported in Unix platforms.

## Installation

`go get cirello.io/runner/v3`

https://pkg.go.dev/cirello.io/runner/v3

