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
	"os/exec"
	"syscall"
	"time"
)

func commandContext(cmd string) (*exec.Cmd, func() error) {
	c := exec.Command("sh", "-c", cmd)
	c.WaitDelay = 1 * time.Minute
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c, func() error {
		if err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL); err != nil {
			return err
		}
		if err := c.Process.Kill(); err != nil {
			return err
		}
		return nil
	}
}
