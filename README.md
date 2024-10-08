# Runner

[![GoDoc](https://pkg.go.dev/badge/cirello.io/runner/v2)](https://pkg.go.dev/cirello.io/runner/v2)
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

- baseport: when set to a number, it will be used as the starting point for
the $PORT environment variable. Each process type will have its own exclusive
$PORT variable value.

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

- signal (in process types): "SIGTERM", "term", or "15" terminates the process;
"SIGKILL", "kill", or "9" kills the process. The default is "SIGKILL".

- timeout (in process types): duration (in Go format) to wait after
sending the signal to the process.

## CLI parameters

```Shell
NAME:
   runner - simple Procfile runner

USAGE:
   runner [global options] command [command options] [arguments...]

VERSION:
   v2 (4207c79dde9478596d3af8e055f00201cc7ddfec)

COMMANDS:
   logs     Follows logs from running processes
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --port value                                         base IP port used to set $PORT for each process type. Should be multiple of 1000. (default: 0)
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

`-port PORT` is the base IP port number used for each process type. It passes
the port number as an environment variable named `$PORT` to the process, and
it can be used as means to facilitate the application start up.

## Environment variables available to processes

Each process will have three environment variables available.

`PS` is the name which the runner has christened the process.

`PORT` is the IP port which the runner has indicated to that instance of a
service to bind itself to.

`DISCOVERY` is the HTTP service that returns a JSON describing each process
type port. This assumes the process has honored the `PORT` variable and bound
itself to the configured one.

### Service discovery by environment variable

Additionally to the basic three variables above, the runner will add another one
for each instance of a process type, like what follows:

```
# format: NAME_#_PORT
formation: web:3 worker:2
web: server back
worker: some-worker

# Extra vars injected for both web.* and worker.*
# WEB_0_PORT=localhost:5000
# WEB_1_PORT=localhost:5001
# WEB_2_PORT=localhost:5002
# WORKER_0_PORT=localhost:5100
# WORKER_1_PORT=localhost:5101
```

These variable names are compliant with the POSIX standards on shells section of
IEEE Std 1003.1-2008 / IEEE POSIX P1003.2/ISO 9945.2 Shell and Tools.
Non-compliant chars are replaced with an underscore (`_`) and name uniqueness is
enforced.

## Support

`runner/v2` is officially supported in Unix platforms only. It may compile on
MS-Windows but it is not going to have the same behavior as in Unix.

## Installation
`go get cirello.io/runner/v2`

https://pkg.go.dev/cirello.io/runner/v2

