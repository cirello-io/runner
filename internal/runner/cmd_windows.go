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

//go:build windows
// +build windows

package runner

import (
	"os"
	"os/exec"
)

func commandContext(cmd string) (*exec.Cmd, func() error) {
	c := exec.Command("cmd", "/c", cmd)
	return c, func() error {
		if err := c.Process.Signal(os.Interrupt); err != nil {
			return err
		}
		if err := c.Process.Kill(); err != nil {
			return err
		}
		return nil
	}
}
