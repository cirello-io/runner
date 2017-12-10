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

// Package runner holds the building blocks for cmd runner.
package runner // import "cirello.io/runner/runner"

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RestartMode defines how a service should restart itself.
type RestartMode string

// ParseRestartMode takes a string and converts to RestartMode
func ParseRestartMode(m string) RestartMode {
	switch strings.ToLower(m) {
	case "yes", "always", "true", "1":
		return Always
	case "fail", "failure", "onfailure", "on-failure", "on_failure":
		return OnFailure
	default:
		return Never
	}
}

// Restart modes
const (
	Always    RestartMode = "yes"
	OnFailure RestartMode = "fail"
	Never     RestartMode = ""
)

// Service is the piece of software you want to start. Cmd accepts multiple
// commands. All commands are executed in order of declaration. The last command
// is considered the call which activates the service. If WaitBefore is defined,
// it will wait for network readiness on the defined target before executing the
// first command. If WaitFor is defined, it will wait for network readiness on
// the defined target before executing the last command.
// Services named as "build" are special, they are executed first in preparation
// for all other services, upon their completion the application initialized.
type Service struct {
	// Name of the service
	Name string

	// Cmd are the commands necessary to start the service. Each command
	// is executed on its own separated shell. No state is shared across
	// commands.
	Cmd []string

	// WaitBefore is the network address that the service waits to be
	// available before initiating the service start.
	WaitBefore string

	// WaitFor is the network address that the service waits to be available
	// before finalizing the start.
	WaitFor string

	// Restart is the flag that forces the service to restart. It means that
	// all steps are executed upon restart. This option does not apply to
	// build steps.
	//
	// - yes|always: alway restart the service.
	// - no|<empty>: never restart the service.
	// - on-failure|fail: restart the service if any of the steps fail.
	Restart RestartMode
}

// Runner defines how this application should be started.
type Runner struct {
	// WorkDir is the working directory from which all commands are going
	// to be executed.
	WorkDir string

	// Observables are the filepath.Match() patterns used to scan for files
	// with changes.
	Observables []string

	// SkipDirs are the directory names that are ignored during changed file
	// scanning.
	SkipDirs []string

	// Services is the list of services necessary to start this application.
	Services []*Service

	longestServiceName int
}

// Start initiates the application.
func (r Runner) Start() error {
	for _, svc := range r.Services {
		if l := len(svc.Name); l > r.longestServiceName {
			r.longestServiceName = l
		}
	}

	updates, err := r.monitorWorkDir()
	if err != nil {
		return err
	}

	for {
		ctx, cancel := context.WithCancel(context.Background())
		go r.startServices(ctx)
		<-updates
		cancel()
	}
}

func (r Runner) startServices(ctx context.Context) {
	var (
		wgBuild    sync.WaitGroup
		mu         sync.Mutex
		anyFailure = false
	)
	for _, sv := range r.Services {
		if !strings.HasPrefix(sv.Name, "build") {
			continue
		}
		wgBuild.Add(1)
		go func(sv *Service) {
			defer wgBuild.Done()
			if !r.startService(ctx, sv) {
				mu.Lock()
				anyFailure = true
				mu.Unlock()
			}
		}(sv)
	}
	wgBuild.Wait()

	if anyFailure {
		log.Println("error during build, halted")
		return
	}

	var wgRun sync.WaitGroup
	for _, sv := range r.Services {
		if strings.HasPrefix(sv.Name, "build") {
			continue
		}
		wgRun.Add(1)
		go func(sv *Service) {
			defer wgRun.Done()
			for {
				ok := r.startService(ctx, sv)
				select {
				case <-ctx.Done():
					return
				default:
				}
				stop := !(sv.Restart == Always ||
					!ok && sv.Restart == OnFailure)
				if stop {
					break
				}
			}
		}(sv)
	}
	wgRun.Wait()
}

func (r Runner) startService(ctx context.Context, sv *Service) bool {
	pr, pw := io.Pipe()
	r.prefixedPrinter(pr, sv.Name)
	defer pw.Close()
	defer pr.Close()
	for idx, cmd := range sv.Cmd {
		fmt.Fprintln(pw, "running", `"`+cmd+`"`)
		c := exec.CommandContext(ctx, "sh", "-c", cmd)
		c.Dir = r.WorkDir
		stderrPipe, err := c.StderrPipe()
		if err != nil {
			fmt.Fprintln(pw, "cannot open stderr pipe", sv.Name, cmd)
			continue
		}
		stdoutPipe, err := c.StdoutPipe()
		if err != nil {
			fmt.Fprintln(pw, "cannot open stdout pipe", sv.Name, cmd)
			continue
		}

		r.prefixedPrinter(stderrPipe, sv.Name)
		r.prefixedPrinter(stdoutPipe, sv.Name)

		isFirstCommand := idx == 0
		isLastCommand := idx+1 == len(sv.Cmd)
		if isFirstCommand && sv.WaitBefore != "" {
			waitFor(ctx, pw, sv.WaitBefore)
		} else if isLastCommand && sv.WaitFor != "" {
			waitFor(ctx, pw, sv.WaitFor)
		}

		if err := c.Run(); err != nil {
			fmt.Fprintf(pw, "exec error %s: (%s) %v\n", sv.Name, cmd, err)
			return false
		}
	}
	return true
}

func waitFor(ctx context.Context, w io.Writer, target string) {
	fmt.Fprintln(w, "waiting for", target)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, err := net.Dial("tcp", target)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		break
	}
	fmt.Fprintln(w, "starting")
}

func (r Runner) prefixedPrinter(rdr io.Reader, name string) *bufio.Scanner {
	paddedName := (name + strings.Repeat(" ", r.longestServiceName))[:r.longestServiceName]
	scanner := bufio.NewScanner(rdr)
	go func() {
		for scanner.Scan() {
			fmt.Println(paddedName+":", scanner.Text())
		}
		if err := scanner.Err(); err != nil && err != os.ErrClosed && err != io.ErrClosedPipe {
			fmt.Println(paddedName+":", "error:", err)
		}
	}()
	return scanner
}
