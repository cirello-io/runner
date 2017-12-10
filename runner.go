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
	// available initiating the service start.
	WaitBefore string

	// WaitFor is the network address that the service waits to be available
	// before finalizing the start.
	WaitFor string
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
}

// Start initiates the application.
func (s Runner) Start() error {
	updates, err := s.monitorWorkDir()
	if err != nil {
		return err
	}

	for {
		ctx, cancel := context.WithCancel(context.Background())
		go s.startServices(ctx)
		<-updates
		cancel()
	}
}

func (s Runner) startServices(ctx context.Context) {
	var (
		wgBuild    sync.WaitGroup
		mu         sync.Mutex
		anyFailure = false
	)
	for _, sv := range s.Services {
		if !strings.HasPrefix(sv.Name, "build") {
			continue
		}
		wgBuild.Add(1)
		go func(sv *Service) {
			defer wgBuild.Done()
			if !startService(ctx, s.WorkDir, sv) {
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
	for _, sv := range s.Services {
		if strings.HasPrefix(sv.Name, "build") {
			continue
		}
		wgRun.Add(1)
		go func(sv *Service) {
			defer wgRun.Done()
			startService(ctx, s.WorkDir, sv)
		}(sv)
	}
	wgRun.Wait()
}

func startService(ctx context.Context, workDir string, sv *Service) bool {
	r, w := io.Pipe()
	prefixedPrinter(r, sv.Name)
	defer w.Close()
	defer r.Close()
	for idx, cmd := range sv.Cmd {
		fmt.Fprintln(w, "running", `"`+cmd+`"`)
		c := exec.CommandContext(ctx, "sh", "-c", cmd)
		c.Dir = workDir
		stderrPipe, err := c.StderrPipe()
		if err != nil {
			fmt.Fprintln(w, "cannot open stderr pipe", sv.Name, cmd)
			continue
		}
		stdoutPipe, err := c.StdoutPipe()
		if err != nil {
			fmt.Fprintln(w, "cannot open stdout pipe", sv.Name, cmd)
			continue
		}

		prefixedPrinter(stderrPipe, sv.Name)
		prefixedPrinter(stdoutPipe, sv.Name)

		isFirstCommand := idx == 0
		isLastCommand := idx+1 == len(sv.Cmd)
		if isFirstCommand && sv.WaitBefore != "" {
			waitFor(ctx, w, sv.WaitBefore)
		} else if isLastCommand && sv.WaitFor != "" {
			waitFor(ctx, w, sv.WaitFor)
		}

		if err := c.Run(); err != nil {
			fmt.Fprintf(w, "exec error %s: (%s) %v\n", sv.Name, cmd, err)
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

func prefixedPrinter(r io.Reader, name string) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	go func() {
		for scanner.Scan() {
			fmt.Println(name+":", scanner.Text())
		}
		if err := scanner.Err(); err != nil && err != os.ErrClosed && err != io.ErrClosedPipe {
			fmt.Println(name+":", "error:", err)
		}
	}()
	return scanner
}
