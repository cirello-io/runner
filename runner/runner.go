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
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	supervisor "cirello.io/supervisor/easy"
)

// ErrNonUniqueProcessTypeName is returned when starting the runner, it detects
// that its configuration possesses non unique process type names after it
// normalizes them.
var ErrNonUniqueProcessTypeName = errors.New("non unique process type name")

// RestartMode defines if a process should restart itself.
type RestartMode string

// ParseRestartMode takes a string and converts to RestartMode. If the parsing
// fails, it silently defaults to Never.
func ParseRestartMode(m string) RestartMode {
	switch strings.ToLower(m) {
	case "yes", "always", "true", "1":
		return Always
	case "fail", "failure", "onfail", "onfailure", "on-failure", "on_failure":
		return OnFailure
	case "temporary", "start-once", "temp", "tmp":
		return Temporary
	default:
		return Never
	}
}

// Restart modes
const (
	Always    RestartMode = "yes"
	OnFailure RestartMode = "fail"
	Temporary RestartMode = "temporary"
	Never     RestartMode = ""
)

// ProcessType is the piece of software you want to start. Cmd accepts multiple
// commands. All commands are executed in order of declaration. The last command
// is considered the call which activates the process type. If WaitBefore is
// defined, it will wait for network readiness on the defined target before
// executing the first command. If WaitFor is defined, it will wait for network
// readiness on the defined target before executing the last command. Process
// types named with prefix "build" are special, they are executed first in
// preparation for all other process types, upon their completion the
// application initialized.
type ProcessType struct {
	// Name of the process type. If the name is prefixed with "build" it is
	// executed before the others.
	Name string `json:"name"`

	// Cmd are the commands necessary to start the process type. They are
	// executed in sequence, each its own separated shell. No state is
	// shared across commands.
	Cmd []string `json:"cmd"`

	// WaitBefore is the network address or process type name that the
	// process type waits to be available before initiating the process type
	// start.
	WaitBefore string `json:"waitbefore,omitempty"`

	// WaitFor is the network address or process type name that the process
	// type waits to be available before finalizing the start.
	WaitFor string `json:"waitfor,omitempty"`

	// Restart is the flag that forces the process type to restart. It means
	// that all steps are executed upon restart. This option does not apply
	// to build steps.
	//
	// - yes|always: alway restart the process type.
	// - no|<empty>: restart the process type on rebuild.
	// - on-failure|fail: restart the process type if any of the steps fail.
	// - temporary|tmp: start the process once and skip restart on rebuild.
	// Temporary processes do not show up in the discovery service.
	Restart RestartMode `json:"restart,omitempty"`

	// Group defines to which supervisor group this process type belongs.
	// Group is useful to contain restart to a subset of the process types.
	Group string

	// Sticky processes are not interrupted by filesystem events.
	Sticky bool
}

// Runner defines how this application should be started.
type Runner struct {
	// WorkDir is the working directory from which all commands are going
	// to be executed.
	WorkDir string `json:"workdir,omitempty"`

	// Observables are the filepath.Match() patterns used to scan for files
	// with changes.
	Observables []string `json:"observables,omitempty"`

	// SkipDirs are the directory names that are ignored during changed file
	// scanning.
	SkipDirs []string `json:"skipdir,omitempty"`

	// Processes is the list of processes necessary to start this
	// application.
	Processes []*ProcessType `json:"procs"`

	// BasePort is the IP port number used to calculate an IP port for each
	// process type and set to its $PORT environment variable. Build
	// processes do not earn an IP port.
	BasePort int

	// Formation allows to start more than one process type each time. Each
	// start will yield its own exclusive $PORT. Formation does not apply
	// to build process types.
	Formation map[string]int // map of process type name and count

	// BaseEnvironment is the set of environment variables loaded into
	// the service.
	BaseEnvironment []string

	longestProcessTypeName int

	// ServiceDiscoveryAddr is the net.Listen address used to bind the
	// service discovery service. Set to empty to disable it. If activated
	// this address is passed to the processes through the environment
	// variable named "DISCOVERY".
	ServiceDiscoveryAddr string

	sdMu                    sync.Mutex
	dynamicServiceDiscovery map[string]string
	staticServiceDiscovery  []string
	currentGeneration       int
}

// New creates a new runner ready to use.
func New() Runner {
	return Runner{
		Formation:               make(map[string]int),
		dynamicServiceDiscovery: make(map[string]string),
	}
}

// Start initiates the application.
func (r *Runner) Start(rootCtx context.Context) error {
	nameDict := make(map[string]struct{})
	for _, proc := range r.Processes {
		name := proc.Name
		if formation, ok := r.Formation[proc.Name]; ok {
			name = fmt.Sprintf("%v.%v", proc.Name, formation)
		} else {
			name = fmt.Sprintf("%v.%v", proc.Name, 0)
		}

		if _, ok := nameDict[normalizeByEnvVarRules(name)]; ok {
			return ErrNonUniqueProcessTypeName
		}
		nameDict[normalizeByEnvVarRules(name)] = struct{}{}

		if l := len(name); l > r.longestProcessTypeName {
			r.longestProcessTypeName = l
		}
	}
	r.longestProcessTypeName++

	go r.serveServiceDiscovery(rootCtx)

	updates, err := r.monitorWorkDir(rootCtx)
	if err != nil {
		return err
	}

	run := make(chan string)
	fileHashes := make(map[string]string) // fn to hash
	c, cancel := context.WithCancel(rootCtx)
	for {
		select {
		case <-rootCtx.Done():
			cancel()
			return nil
		case fn := <-updates:
			newHash := calcFileHash(fn)
			oldHash, ok := fileHashes[fn]
			if ok && newHash == oldHash && len(updates) > 0 {
				log.Println(fn, "didn't change, skipping")
				continue
			}
			fileHashes[fn] = newHash

			if ok := r.runBuilds(c, fn); !ok {
				log.Println("error during build, halted")
				continue
			}

			if l := len(updates); l == 0 {
				cancel()
				go func() { run <- fn }()
			} else {
				log.Println("builds pending before application start:", l)
			}
		case fn := <-run:
			c, cancel = context.WithCancel(rootCtx)
			go r.runNonBuilds(rootCtx, c, fn)
		}
	}
}

func calcFileHash(fn string) string {
	f, err := os.Open(fn)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

func (r *Runner) runBuilds(ctx context.Context, fn string) bool {
	var (
		wgBuild sync.WaitGroup
		mu      sync.Mutex
		ok      = true
	)
	for _, sv := range r.Processes {
		if !strings.HasPrefix(sv.Name, "build") {
			continue
		}
		r.setServiceDiscovery(normalizeByEnvVarRules(sv.Name), "building")
		wgBuild.Add(1)
		go func(sv *ProcessType) {
			defer wgBuild.Done()
			defer func() {
				r.setServiceDiscovery(normalizeByEnvVarRules(sv.Name), "done")
			}()
			c := ctx
			if sv.Sticky {
				log.Println(sv.Name, "is sticky")
				c = context.Background()
			}
			if !r.startProcess(c, sv, -1, -1, fn) {
				mu.Lock()
				ok = false
				mu.Unlock()
			}
		}(sv)
	}
	wgBuild.Wait()
	return ok
}

func (r *Runner) runNonBuilds(rootCtx, ctx context.Context, changedFileName string) {
	ctx = supervisor.WithContext(ctx)
	groups := make(map[string]context.Context)
	ready := make(chan struct{})

	for j, sv := range r.Processes {
		if strings.HasPrefix(sv.Name, "build") {
			continue
		}

		maxProc := 1
		if formation, ok := r.Formation[sv.Name]; ok {
			maxProc = formation
		}

		procCtx := ctx
		if sv.Group != "" {
			groupCtx, ok := groups[sv.Group]
			if !ok {
				groupCtx = supervisor.WithContext(ctx)
				groups[sv.Group] = groupCtx
			}
			procCtx = groupCtx
		}

		portCount := j * 100
		for i := 0; i < maxProc; i++ {
			sv, i, pc := sv, i, portCount

			if sv.Restart == Temporary && r.currentGeneration == 0 {
				temporarySvcCtx := supervisor.WithContext(rootCtx)
				supervisor.Add(temporarySvcCtx, func(ctx context.Context) {
					<-ready
					r.startProcess(ctx, sv, i, pc, changedFileName)
				}, supervisor.Temporary)
				portCount++
			} else if sv.Restart == Temporary && r.currentGeneration != 0 {
				portCount++
				continue
			} else {
				opt := supervisor.Temporary
				switch sv.Restart {
				case Always:
					opt = supervisor.Permanent
				case OnFailure:
					opt = supervisor.Transient
				}
				supervisor.Add(procCtx, func(ctx context.Context) {
					<-ready
					ok := r.startProcess(ctx, sv, i, pc, changedFileName)
					if !ok && sv.Restart == OnFailure {
						panic("restarting on failure")
					}
				}, opt)
				r.staticServiceDiscovery = append(
					r.staticServiceDiscovery,
					fmt.Sprintf("%s=localhost:%d", discoveryEnvVar(sv.Name, i), r.BasePort+portCount),
				)
				portCount++
			}
		}
	}
	r.currentGeneration++
	close(ready)

	<-ctx.Done()
}

func discoveryEnvVar(name string, procCount int) string {
	return normalizeByEnvVarRules(fmt.Sprintf("%s_%d_PORT", name, procCount))
}

// normalizeByEnvVarRules takes any name and rewrites it to be compliant with
// the POSIX standards on shells section of IEEE Std 1003.1-2008 / IEEE POSIX
// P1003.2/ISO 9945.2 Shell and Tools standard.
func normalizeByEnvVarRules(name string) string {
	//[a-zA-Z_]+[a-zA-Z0-9_]*
	var buf bytes.Buffer
	for i, v := range name {
		switch {
		case i == 0 && v >= '0' && v <= '9',
			!(v >= 'a' && v <= 'z' || v >= 'A' && v <= 'Z' || v >= '0' && v <= '9'):
			buf.WriteRune('_')
			continue
		}
		buf.WriteRune(v)
	}
	return strings.ToUpper(buf.String())
}

func (r *Runner) startProcess(ctx context.Context, sv *ProcessType, procCount, portCount int, changedFileName string) bool {
	pr, pw := io.Pipe()
	procName := sv.Name
	port := r.BasePort + portCount
	if procCount > -1 {
		procName = fmt.Sprintf("%v.%v", procName, procCount)
	}
	if portCount > -1 {
		r.setServiceDiscovery(discoveryEnvVar(sv.Name, procCount), fmt.Sprint("localhost:", port))
	}
	r.prefixedPrinter(ctx, pr, procName)

	defer pw.Close()
	defer pr.Close()

	for idx, cmd := range sv.Cmd {
		fmt.Fprintln(pw, "running", `"`+cmd+`"`)
		defer fmt.Fprintln(pw, "finished", `"`+cmd+`"`)
		if portCount > -1 {
			fmt.Fprintln(pw, "listening on", port)
		}
		fmt.Fprintln(pw)
		c := exec.CommandContext(ctx, "sh", "-c", cmd)
		c.Dir = r.WorkDir

		c.Env = os.Environ()
		if len(r.BaseEnvironment) > 0 {
			c.Env = r.BaseEnvironment
		}
		c.Env = append(c.Env, fmt.Sprintf("PS=%v", procName))
		if portCount > -1 {
			c.Env = append(c.Env, fmt.Sprintf("PORT=%d", port))
		}

		if r.ServiceDiscoveryAddr != "" {
			c.Env = append(c.Env, fmt.Sprintf("DISCOVERY=%v", r.ServiceDiscoveryAddr))
			c.Env = append(c.Env, r.staticServiceDiscovery...)
		}

		c.Env = append(c.Env, fmt.Sprintf("CHANGED_FILENAME=%v", changedFileName))

		stderrPipe, err := c.StderrPipe()
		if err != nil {
			fmt.Fprintln(pw, "cannot open stderr pipe", procName, cmd)
			continue
		}
		stdoutPipe, err := c.StdoutPipe()
		if err != nil {
			fmt.Fprintln(pw, "cannot open stdout pipe", procName, cmd)
			continue
		}

		r.prefixedPrinter(ctx, stderrPipe, procName)
		r.prefixedPrinter(ctx, stdoutPipe, procName)

		isFirstCommand := idx == 0
		isLastCommand := idx+1 == len(sv.Cmd)
		if isFirstCommand && sv.WaitBefore != "" {
			r.waitFor(ctx, pw, sv.WaitBefore)
		} else if isLastCommand && sv.WaitFor != "" {
			r.waitFor(ctx, pw, sv.WaitFor)
		}

		if err := c.Run(); err != nil {
			fmt.Fprintf(pw, "exec error %s: (%s) %v\n", procName, cmd, err)
			return false
		}
	}
	return true
}

func (r *Runner) waitFor(ctx context.Context, w io.Writer, target string) {
	fmt.Fprintln(w, "waiting for", target)
	defer fmt.Fprintln(w, "starting")
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
			target = r.resolveProcessTypeAddress(target)
			c, err := net.Dial("tcp", target)
			if err == nil {
				c.Close()
				return
			}
		}
	}
}

func (r *Runner) resolveProcessTypeAddress(target string) string {
	r.sdMu.Lock()
	defer r.sdMu.Unlock()

	for name, port := range r.dynamicServiceDiscovery {
		if strings.HasPrefix(name, target) {
			return fmt.Sprint("localhost:", port)
		}
	}
	return target
}

func (r *Runner) prefixedPrinter(ctx context.Context, rdr io.Reader, name string) *bufio.Scanner {
	paddedName := (name + strings.Repeat(" ", r.longestProcessTypeName))[:r.longestProcessTypeName]
	scanner := bufio.NewScanner(rdr)
	scanner.Buffer(make([]byte, 65536), 2*1048576)
	go func() {
		for scanner.Scan() {
			fmt.Println(paddedName+":", scanner.Text())
		}

		select {
		// If the context is cancelled, we really don't care about
		// errors anymore.
		case <-ctx.Done():
			return
		default:
			if err := scanner.Err(); err != nil && err != os.ErrClosed && err != io.ErrClosedPipe {
				fmt.Println(paddedName+":", "error:", err)
			}
		}
	}()
	return scanner
}

func (r *Runner) setServiceDiscovery(svc, state string) {
	r.sdMu.Lock()
	r.dynamicServiceDiscovery[svc] = state
	r.sdMu.Unlock()
}
