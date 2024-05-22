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

package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func parseEnvFile(r io.Reader) ([]string, error) {
	var env []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		commentIdx := strings.Index(line, "#")
		if commentIdx != -1 {
			line = strings.TrimSpace(line[0:commentIdx])
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.ToLower(key), "export ") {
			key = key[7:]
		}
		if len(value) > 0 && value[0] == '"' {
			v, err := strconv.Unquote(value)
			if err != nil {
				continue
			}
			value = v
		}
		env = append(env, fmt.Sprintf("%v=%v", key, value))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}
