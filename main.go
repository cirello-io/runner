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

/*
Command runner is a very ugly and simple structured command executer that
monitor file changes to trigger process restarts.

Create a file name Procfile in the root of the project you want to run, and add
the following content:

	workdir: $GOPATH/src/github.com/example/go-app
	observe: *.go *.js
	ignore: /vendor
	build-server: make server
	web-a: group=web restart=onbuild waitfor=localhost:8888 ./server serve alpha
	web-b: group=web restart=onbuild waitfor=localhost:8888 ./server serve bravo
	db: restart=failure waitfor=localhost:8888 ./server db

Special process types:

- workdir: the working directory. Environment variables are expanded. It follows
the same rules for exec.Command.Dir.

- observe: a space separated list of file patterns to scan for. It uses
filepath.Match internally. File patterns preceded with exclamation mark (!) will
not trigger builds.

- ignore: a space separated list of ignored directories relative to workdir,
typically vendor directories.

- build*: process type name prefixed by "build" are always executed first and in
order of declaration. On failure, they halt the initialization.

- waitfor (in process type): target hostname and port that the runner will probe
before starting the process type.

- restart (in process type): "onbuild" will restart the process type at every
build; "fail" will restart the process type on failure; "loop" restart the
process when it naturally terminates; "temporary" runs the process only once.

- group (in process type): group of processes that depend on each other. If a
process type fails, it will halt all others in the same group. If the
"restart" parameter is not set to "always" or "fail", the affected process
types will halt and not restart.

- signal (in process types): "SIGTERM", "term", or "15" terminates the process;
"SIGKILL", "kill", or "9" kills the process. The default is "SIGKILL".

- signalTimeout (in process types): duration (in Go format) to wait after
sending the signal to the process.

- optional (in process types): does not start this process unless explicit told
so. The process type must be part of a group.
*/
package main // import "cirello.io/runner/v2"

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"cirello.io/runner/v2/internal/envfile"
	"cirello.io/runner/v2/internal/procfile"
	"cirello.io/runner/v2/internal/runner"
	cli "github.com/urfave/cli"
)

const defaultProcfile = "Procfile"

func main() {
	log.SetFlags(0)
	log.SetPrefix("runner: ")
	app := cli.NewApp()
	app.Name = "runner"
	app.Usage = "simple Procfile runner"
	app.HideVersion = true
	app.EnableBashCompletion = false
	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port",
			Value: 0,
			Usage: "base IP port used to set $PORT for each process type. Should be multiple of 1000.",
		},
		cli.StringFlag{
			Name:  "service-discovery",
			Value: "localhost:64000",
			Usage: "service discovery address",
		},
		cli.StringFlag{
			Name:  "formation",
			Value: "",
			Usage: "formation allows to start more than one instance of a process type, format: `procTypeA=# procTypeB=# ... procTypeN=#`",
		},
		cli.StringFlag{
			Name:  "env",
			Value: ".env",
			Usage: "environment `file` to be loaded for all processes, if the file is absent, then this parameter is ignored.",
		},
		cli.StringFlag{
			Name:  "skip",
			Value: "",
			Usage: "does not run some of the process types, format: `procTypeA procTypeB procTypeN`",
		},
		cli.StringFlag{
			Name:  "only",
			Value: "",
			Usage: "only runs some of the process types, format: `procTypeA procTypeB procTypeN`",
		},
		cli.StringFlag{
			Name:  "optional",
			Value: "",
			Usage: "forcefully runs some of the process types, format: `procTypeA procTypeB procTypeN`",
		},
	}
	app.Commands = []cli.Command{logs()}
	app.Action = mainRunner
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func mainRunner(c *cli.Context) error {
	origStdout := os.Stdout
	basePort := c.Int("port")
	envFN := c.String("env")
	skipProcs := c.String("skip")
	onlyProcs := c.String("only")
	optionalProcs := c.String("optional")
	discoveryAddr := c.String("service-discovery")
	formation := c.String("formation")

	var (
		filterPatternMu sync.RWMutex
		filterPattern   string
	)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := scanner.Text()
			filterPatternMu.Lock()
			filterPattern = text
			filterPatternMu.Unlock()
			if text != "" {
				log.Println("filtering with:", scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard output:", err)
		}
	}()
	r, w, _ := os.Pipe()
	os.Stdout = w
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 2097152), 262144)
		for scanner.Scan() {
			filterPatternMu.RLock()
			pattern := filterPattern
			filterPatternMu.RUnlock()
			text := scanner.Text()
			if pattern == "" {
				fmt.Fprintln(origStdout, text)
				continue
			}
			if strings.Contains(text, pattern) {
				fmt.Fprintln(origStdout, text)
				break
			}
		}
		if err := scanner.Err(); err != nil {
			log.Println("reading standard output:", err)
		}
	}()

	fn := defaultProcfile
	if argFn := c.Args().First(); argFn != "" {
		fn = argFn
	}

	fd, err := os.Open(fn)
	if err != nil {
		return err
	}

	if basePort != 0 && (basePort < 1 || basePort > 65535) {
		return errors.New("invalid IP port")
	}

	s, err := procfile.Parse(fd)
	if err != nil {
		return fmt.Errorf("cannot parse spec file (procfile): %v", err)
	}
	if len(s.Formation) == 0 && formation != "" {
		s.Formation = procfile.ParseFormation(formation)
	}

	s.WorkDir = os.ExpandEnv(s.WorkDir)
	if s.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot load current workdir: %v", err)
		}
		s.WorkDir = wd
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s.BasePort = basePort

	if envFN != "" {
		fd, err := os.Open(envFN)
		if err == nil {
			baseEnv, err := envfile.Parse(fd)
			if err != nil {
				return fmt.Errorf("error reading environment file (%v): %v", envFN, err)
			}
			if err := fd.Close(); err != nil {
				return fmt.Errorf("cannot close environment file reader (%v): %v", envFN, err)
			}
			s.BaseEnvironment = baseEnv
		}
	}

	if skipProcs != "" {
		s.Processes = filterSkippedProcs(skipProcs, s.Processes)
	} else if onlyProcs != "" {
		s.Processes = filterOnlyProcs(onlyProcs, s.Processes)
	}
	if optionalProcs != "" {
		s.Processes = keepOptionalProcs(optionalProcs, s.Processes)
	} else {
		s.Processes = filterOptionalProcs(s.Processes)
	}
	s.ServiceDiscoveryAddr = discoveryAddr
	if err := s.Start(ctx); err != nil {
		return fmt.Errorf("cannot serve: %v", err)
	}
	return nil
}

func keepOptionalProcs(optionals string, processes []*runner.ProcessType) []*runner.ProcessType {
	optionalProcs, newProcs := strings.Split(optionals, " "), []*runner.ProcessType{}
procTypes:
	for _, procType := range processes {
		if !procType.Optional {
			newProcs = append(newProcs, procType)
			continue
		}
		for _, optional := range optionalProcs {
			if procType.Name == optional {
				fmt.Println("enabling", optional)
				newProcs = append(newProcs, procType)
				continue procTypes
			}
		}
	}
	return newProcs
}

func filterOptionalProcs(processes []*runner.ProcessType) []*runner.ProcessType {
	groups := make(map[string]struct{})
	newProcs := []*runner.ProcessType{}
	for _, procType := range processes {
		if procType.Optional && procType.Group == "" {
			continue
		} else if procType.Optional && procType.Group != "" {
			if _, ok := groups[procType.Group]; ok {
				continue
			}
			groups[procType.Group] = struct{}{}
		}
		newProcs = append(newProcs, procType)
	}
	return newProcs
}

func filterSkippedProcs(skip string, processes []*runner.ProcessType) []*runner.ProcessType {
	skipProcs, newProcs := strings.Split(skip, " "), []*runner.ProcessType{}
procTypes:
	for _, procType := range processes {
		for _, skip := range skipProcs {
			if procType.Name == skip {
				fmt.Println("skipping", skip)
				continue procTypes
			}
		}
		newProcs = append(newProcs, procType)
	}
	return newProcs
}

func filterOnlyProcs(only string, processes []*runner.ProcessType) []*runner.ProcessType {
	onlyProcs, newProcs := strings.Split(only, " "), []*runner.ProcessType{}
procTypes:
	for _, procType := range processes {
		for _, only := range onlyProcs {
			if procType.Name == only {
				newProcs = append(newProcs, procType)
				continue procTypes
			}
		}
	}
	return newProcs
}

func logs() cli.Command {
	return cli.Command{
		Name:  "logs",
		Usage: "Follows logs from running processes",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "filter",
				Usage: "service name to filter message",
			},
		},
		Action: func(c *cli.Context) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			u := url.URL{Scheme: "http", Host: c.GlobalString("service-discovery"), Path: "/logs"}
			if filter := c.String("filter"); filter != "" {
				query := u.Query()
				query.Set("filter", filter)
				u.RawQuery = query.Encode()
			}
			log.Printf("connecting to %s", u.String())
			follow := func() (outErr error) {
				req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
				if err != nil {
					return fmt.Errorf("cannot create request: %v", err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("cannot connect to service discovery endpoint: %v", err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("bad status: %s", resp.Status)
				}
				br := bufio.NewReaderSize(resp.Body, 10*1024*1024)
				for {
					if ctx.Err() != nil {
						return nil
					}
					l, partialRead, err := br.ReadLine()
					if errors.Is(err, io.EOF) {
						return nil
					} else if err != nil {
						return err
					} else if partialRead {
						return fmt.Errorf("partial read: %v", string(l))
					}
					l = bytes.TrimSpace(bytes.TrimPrefix(l, []byte("data: ")))
					if len(l) == 0 {
						continue
					}
					var msg runner.LogMessage
					if err := json.Unmarshal(l, &msg); err != nil {
						log.Println("decode:", err)
						return err
					}
					fmt.Println(msg.PaddedName+":", msg.Line)
				}
			}
			var errFollow error
			for {
				if ctx.Err() != nil {
					return errFollow
				}
				errFollow = follow()
			}
		},
	}
}
