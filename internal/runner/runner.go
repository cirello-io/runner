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

// Package runner holds the building blocks for cmd runner.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"cirello.io/oversight"
	"cirello.io/takelatest"
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
	case "onbuild", "yes", "always", "true", "1", "build":
		return OnBuild
	case "fail", "failure", "onfail", "onfailure", "on-failure", "on_failure":
		return OnFailure
	case "temporary", "start-once", "temp", "tmp":
		return Temporary
	case "loop":
		return Loop
	default:
		return Never
	}
}

// Restart modes
const (
	OnBuild   RestartMode = "onbuild"
	OnFailure RestartMode = "fail"
	Temporary RestartMode = "temporary"
	Loop      RestartMode = "loop"
	Never     RestartMode = ""
)

// ProcessType is the piece of software you want to start. Cmd accepts multiple
// commands. All commands are executed in order of declaration. The last command
// is considered the call which activates the process type. If WaitFor is
// defined, it will wait for network readiness on the defined target before
// executing the last command. Process types named with prefix "build" are
// special, they are executed first in preparation for all other process types,
// upon their completion the application initialized.
type ProcessType struct {
	// Name of the process type. If the name is prefixed with "build" it is
	// executed before the others.
	Name string

	// Cmd is the command necessary to start the process type.
	Cmd string

	// WaitFor is the network address or process type name that the process
	// type waits to be available before finalizing the start.
	WaitFor string

	// Restart is the flag that forces the process type to restart. It means
	// that all steps are executed upon restart. This option does not apply
	// to build steps.
	//
	// - yes|onbuild: alway restart the process type.
	// - no|<empty>: restart the process type on rebuild.
	// - on-failure|fail: restart the process type if any of the steps fail.
	// - temporary|tmp: start the process once and skip restart on rebuild.
	// Temporary processes do not show up in the discovery service.
	Restart RestartMode `json:"restart,omitempty"`
}

// Runner defines how this application should be started.
type Runner struct {
	// WorkDir is the working directory from which all commands are going
	// to be executed.
	WorkDir string

	// Observables are the filepath.Match() patterns used to scan for files
	// with changes. File patterns preceded with exclamation mark (!) will
	// not trigger builds.
	Observables []string

	// SkipDirs are the directory names that are ignored during changed file
	// scanning.
	SkipDirs []string

	// Processes is the list of processes necessary to start this
	// application.
	Processes []*ProcessType

	// Formation allows to start more than one process type each time. Each
	// start will yield its own exclusive $PORT. Formation does not apply
	// to build process types.
	Formation map[string]int // map of process type name and count

	// SkipProcs is the list of process types that should not be started.
	// This is useful to disable a process type that is not necessary for
	// the current environment.
	SkipProcs []string

	// BaseEnvironment is the set of environment variables loaded into
	// the service.
	BaseEnvironment []string

	longestProcessTypeName int

	// ServiceDiscoveryAddr is the net.Listen address used to bind the
	// service discovery service. Set to empty to disable it. If activated
	// this address is passed to the processes through the environment
	// variable named "DISCOVERY".
	ServiceDiscoveryAddr string

	servicesMu    sync.Mutex
	serviceStates map[string]string // map of service name and state

	logsMu         sync.RWMutex
	logs           chan LogMessage
	logSubscribers []chan LogMessage
}

// LogMessage broadcasted through websocket.
type LogMessage struct {
	PaddedName string `json:"paddedName"`
	Name       string `json:"name"`
	Line       string `json:"line"`
}

// New creates a new runner ready to use.
func New() *Runner {
	return &Runner{
		Formation:     make(map[string]int),
		serviceStates: make(map[string]string),
		logs:          make(chan LogMessage, sseLogForwarderBufferSize),
	}
}

// Start initiates the application.
func (r *Runner) Start(rootCtx context.Context) error {
	slices.SortStableFunc(r.Observables, func(a, b string) int {
		negateA := len(a) > 0 && a[0] == '!'
		negateB := len(b) > 0 && b[0] == '!'
		switch {
		case negateA && !negateB:
			return -1
		case !negateA && negateB:
			return 1
		default:
			return 0
		}
	})
	nameDict := make(map[string]struct{})
	for _, proc := range r.Processes {
		name := fmt.Sprintf("%v.%v", proc.Name, r.Formation[proc.Name])
		if _, ok := nameDict[normalizeByEnvVarRules(name)]; ok {
			return ErrNonUniqueProcessTypeName
		}
		nameDict[normalizeByEnvVarRules(name)] = struct{}{}
		if l := len(name); l > r.longestProcessTypeName {
			r.longestProcessTypeName = l
		}
	}
	r.longestProcessTypeName++
	if err := r.serveWeb(rootCtx); err != nil {
		return fmt.Errorf("cannot serve discovery interface: %w", err)
	}
	r.forwardLogs()
	var (
		runCancel context.CancelFunc = func() {}
		wg        sync.WaitGroup
	)
	ephemeralOnce := sync.OnceFunc(func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.runEphemeral(rootCtx, "")
		}()
	})
	type update struct {
		ctx context.Context
		fn  string
	}
	lastflightRun := &takelatest.Runner[update]{
		Func: func(ctx context.Context, update update) {
			if ctx.Err() != nil {
				return
			}
			wg.Add(1)
			defer wg.Done()
			if ok := r.runBuilds(update.ctx, update.fn); !ok {
				log.Println("error during build, halted")
				return
			}
			ephemeralOnce()
			tree := r.runPermanent(update.fn)
			err := tree.Start(update.ctx)
			log.Println("supervision tree ended", err)
		},
	}
	defer lastflightRun.Close()
	updates := r.monitorWorkDir(rootCtx)
	for {
		select {
		case <-rootCtx.Done():
			runCancel()
			wg.Wait()
			return nil
		case fn := <-updates:
			runCancel()
			ctx, cancel := context.WithCancel(rootCtx)
			runCancel = cancel
			lastflightRun.Take(update{ctx, fn})
		}
	}
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
		maxProc := r.Formation[sv.Name]
		for i := 0; i < maxProc; i++ {
			r.setServiceState(normalizeByEnvVarRules(sv.Name), "building")
			wgBuild.Add(1)
			go func(sv *ProcessType) {
				var buf bytes.Buffer
				localOk := true
				defer wgBuild.Done()
				defer func() {
					status := "done"
					if !localOk {
						status = "errored"
						r.setServiceState("ERROR_"+normalizeByEnvVarRules(sv.Name), buf.String())
					} else {
						r.deleteServiceState("ERROR_" + normalizeByEnvVarRules(sv.Name))
					}
					r.setServiceState(normalizeByEnvVarRules(sv.Name), status)
				}()
				if !r.startProcess(ctx, sv, -1, -1, fn, &buf) {
					mu.Lock()
					ok = false
					localOk = false
					mu.Unlock()
				}
			}(sv)
		}
	}
	wgBuild.Wait()
	return ok
}

func (r *Runner) runPermanent(changedFileName string) *oversight.Tree {
	tree := oversight.New(
		oversight.WithRestartStrategy(oversight.OneForAll()),
		oversight.NeverHalt())
	for j, sv := range r.Processes {
		if strings.HasPrefix(sv.Name, "build") {
			continue
		}
		maxProc := r.Formation[sv.Name]
		portCount := j * 100
		for i := 0; i < maxProc; i++ {
			sv, i, pc := sv, i, portCount
			if sv.Restart == Loop || sv.Restart == Temporary || sv.Restart == OnFailure {
				continue
			}
			_ = tree.Add(oversight.ChildProcessSpecification{
				Name:    sv.Name,
				Restart: oversight.Permanent(),
				Start: func(ctx context.Context) error {
					ok := r.startProcess(ctx, sv, i, pc, changedFileName, io.Discard)
					if !ok && sv.Restart == OnFailure {
						return errors.New("restarting on failure")
					}
					return nil
				},
			})
			portCount++
		}
	}
	return tree
}

func (r *Runner) runEphemeral(ctx context.Context, changedFileName string) {
	tree := oversight.New(
		oversight.WithRestartStrategy(oversight.OneForAll()),
		oversight.NeverHalt())
	for j, sv := range r.Processes {
		if strings.HasPrefix(sv.Name, "build") {
			continue
		}
		maxProc := r.Formation[sv.Name]
		portCount := j * 100
		for i := 0; i < maxProc; i++ {
			sv, i, pc := sv, i, portCount
			if sv.Restart == Loop {
				_ = tree.Add(oversight.ChildProcessSpecification{
					Name:    sv.Name,
					Restart: oversight.Permanent(),
					Start: func(ctx context.Context) error {
						r.startProcess(ctx, sv, i, pc, changedFileName, io.Discard)
						return nil
					},
				})
				portCount++
			} else if sv.Restart == Temporary {
				_ = tree.Add(oversight.ChildProcessSpecification{
					Name:    sv.Name,
					Restart: oversight.Temporary(),
					Start: func(ctx context.Context) error {
						r.startProcess(ctx, sv, i, pc, changedFileName, io.Discard)
						return nil
					},
				})
				portCount++
			} else if sv.Restart == OnFailure {
				_ = tree.Add(oversight.ChildProcessSpecification{
					Name:    sv.Name,
					Restart: oversight.Transient(),
					Start: func(ctx context.Context) error {
						r.startProcess(ctx, sv, i, pc, changedFileName, io.Discard)
						return nil
					},
				})
				portCount++
			}
		}
	}
	_ = tree.Start(ctx)
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

func (r *Runner) startProcess(ctx context.Context, sv *ProcessType, procCount, portCount int, changedFileName string, buf io.Writer) bool {
	pr, pw := io.Pipe()
	procName := sv.Name
	if procCount > -1 {
		procName = fmt.Sprintf("%v.%v", procName, procCount)
	}
	r.prefixedPrinter(ctx, pr, procName)
	defer pw.Close()
	defer pr.Close()
	fmt.Fprintln(pw, "running", `"`+sv.Cmd+`"`)
	defer fmt.Fprintln(pw, "finished", `"`+sv.Cmd+`"`)
	fmt.Fprintln(pw)
	c := command(ctx, sv.Cmd)
	c.Dir = r.WorkDir
	c.Env = os.Environ()
	if len(r.BaseEnvironment) > 0 {
		c.Env = append(c.Env, r.BaseEnvironment...)
	}
	c.Env = append(c.Env, fmt.Sprintf("PS=%v", procName))
	if r.ServiceDiscoveryAddr != "" {
		c.Env = append(c.Env, fmt.Sprintf("DISCOVERY=%v", r.ServiceDiscoveryAddr))
	}
	c.Env = append(c.Env, fmt.Sprintf("CHANGED_FILENAME=%v", changedFileName))
	stderrPipe, err := c.StderrPipe()
	if err != nil {
		fmt.Fprintln(pw, "cannot open stderr pipe", procName, sv.Cmd, err)
		return false
	}
	stdoutPipe, err := c.StdoutPipe()
	if err != nil {
		fmt.Fprintln(pw, "cannot open stdout pipe", procName, sv.Cmd, err)
		return false
	}
	r.prefixedPrinter(ctx, io.TeeReader(stderrPipe, buf), procName)
	r.prefixedPrinter(ctx, io.TeeReader(stdoutPipe, buf), procName)
	if sv.WaitFor != "" {
		r.waitFor(ctx, pw, sv.WaitFor)
	}
	if err := c.Run(); err != nil {
		fmt.Fprintf(pw, "exec error %s: (%s) %v\n", procName, sv.Cmd, err)
		return false
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
			c, err := net.Dial("tcp", target)
			if err == nil {
				c.Close()
				return
			}
		}
	}
}

func (r *Runner) prefixedPrinter(ctx context.Context, rdr io.Reader, name string) *bufio.Scanner {
	paddedName := (name + strings.Repeat(" ", r.longestProcessTypeName))[:r.longestProcessTypeName]
	scanner := bufio.NewScanner(rdr)
	scanner.Buffer(make([]byte, 65536), 2*1048576)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(paddedName+":", line)
			r.logs <- LogMessage{
				PaddedName: paddedName,
				Name:       name,
				Line:       line,
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err := scanner.Err(); err != nil && err != os.ErrClosed && err != io.ErrClosedPipe {
			fmt.Println(paddedName+":", "error:", err)
		}
	}()
	return scanner
}

func (r *Runner) setServiceState(svc, state string) {
	r.servicesMu.Lock()
	r.serviceStates[svc] = state
	r.servicesMu.Unlock()
}

func (r *Runner) deleteServiceState(svc string) {
	r.servicesMu.Lock()
	delete(r.serviceStates, svc)
	r.servicesMu.Unlock()
}

func (s *Runner) monitorWorkDir(ctx context.Context) <-chan string {
	if isValidGitDir(s.WorkDir) {
		log.Println("observing git directory for changes")
		return s.monitorGitDir(ctx, s.WorkDir)
	}
	return s.monitorWorkDirScanner(ctx)
}

func isValidGitDir(dir string) bool {
	err := exec.Command("git", "-C", dir, "--no-optional-locks", "status").Run()
	return err == nil
}

func (s *Runner) monitorGitDir(ctx context.Context, dir string) <-chan string {
	triggereds := make(chan string, 1)
	triggereds <- ""
	go func() {
		defer close(triggereds)
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		memo := make(map[string]time.Time)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			cmd := exec.CommandContext(ctx, "git", "-C", dir, "--no-optional-locks", "status", "--porcelain=v1")
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err != nil {
				log.Println("cannot run git status:", err)
				continue
			}
			var gitfiles []string
			scanner := bufio.NewScanner(&out)
			for scanner.Scan() {
				line := scanner.Text()
				if len(line) < 4 || line[0] == '#' {
					continue
				}
				path := line[3:]
				if path == "" {
					continue
				}
				gitfiles = append(gitfiles, path)
			}
			files := slices.Concat(
				gitfiles,
				slices.Collect(maps.Keys(memo)),
			)
			slices.Sort(files)
			files = slices.Compact(files)
		filesLoop:
			for _, path := range files {
				for _, skipDir := range s.SkipDirs {
					if skipDir == "" {
						continue
					}
					if strings.HasPrefix(path, skipDir) {
						continue filesLoop
					}
				}
				for _, p := range s.Observables {
					if !match(p, path) {
						continue
					}
					info, err := os.Stat(filepath.Join(s.WorkDir, path))
					if err != nil {
						delete(memo, path)
						continue
					}
					mtime := info.ModTime()
					memoMTime, ok := memo[path]
					if !ok {
						memo[path] = mtime
						memoMTime = mtime
					}
					if mtime.Equal(memoMTime) {
						continue
					}
					memo[path] = mtime
					triggereds <- path
				}
			}
		}
	}()
	return triggereds
}

func (s *Runner) monitorWorkDirScanner(ctx context.Context) <-chan string {
	triggereds := make(chan string, 1)
	triggereds <- ""
	go func() {
		defer close(triggereds)
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		memo := make(map[string]time.Time)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			_ = filepath.Walk(s.WorkDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					for _, skipDir := range s.SkipDirs {
						if skipDir == "" {
							continue
						}
						if strings.HasPrefix(path, filepath.Join(s.WorkDir, skipDir)) {
							return filepath.SkipDir
						}
					}
					return nil
				}
				for _, p := range s.Observables {
					if !match(p, path) {
						continue
					}
					mtime := info.ModTime()
					memoMTime, ok := memo[path]
					if !ok {
						memo[path] = mtime
						memoMTime = mtime
					}
					if mtime.Equal(memoMTime) {
						continue
					}
					memo[path] = mtime
					triggereds <- path
				}
				return nil
			})
		}
	}()
	return triggereds
}

func match(p, path string) bool {
	base, dir := filepath.Base(path), filepath.Dir(path)
	pbase, pdir := filepath.Base(p), filepath.Dir(p)
	if matched, err := filepath.Match(pbase, base); err != nil || !matched {
		return false
	}
	if pdir == "." {
		return true
	}
	subpatterns := strings.Split(pdir, "**")
	tmp := dir
	for _, subp := range subpatterns {
		if subp == "" {
			continue
		}
		subp = filepath.Clean(subp)
		t := strings.Replace(tmp, subp, "", 1)
		if t == tmp {
			return false
		}
		tmp = t
	}
	return true
}

func command(ctx context.Context, cmd string) *exec.Cmd {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		pgid := -c.Process.Pid
		osSignal := syscall.SIGTERM
		if err := c.Process.Signal(osSignal); err != nil {
			return fmt.Errorf("cannot signal process: %w", err)
		}
		if err := syscall.Kill(pgid, osSignal); err != nil {
			return fmt.Errorf("cannot signal process group: %w", err)
		}
		c.Process.Wait()
		return nil
	}
	return c
}
