// Copyright 2024 github.com/ucirello
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

//go:build !windows
// +build !windows

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func command(ctx context.Context, cmd string, signal Signal, signalWait time.Duration) *exec.Cmd {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		pgid := -c.Process.Pid
		osSignal := syscall.SIGKILL
		if signal == SignalTERM {
			osSignal = syscall.SIGTERM
		}
		if err := c.Process.Signal(osSignal); err != nil {
			return fmt.Errorf("cannot signal process: %w", err)
		}
		if err := syscall.Kill(pgid, osSignal); err != nil {
			return fmt.Errorf("cannot signal process group: %w", err)
		}
		if signalWait > 0 {
			time.Sleep(signalWait)
		}
		return nil
	}
	return c
}
