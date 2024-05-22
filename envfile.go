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
	"errors"
	"fmt"
	"io"
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
		r := bufio.NewReader(strings.NewReader(line))
		key, err := r.ReadString('=')
		if err != nil {
			continue
		}
		key = key[:len(key)-1]
		if strings.Contains(key, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(key), "export ") {
			key = key[7:]
		}
		var (
			value         string
			isEscaped     bool
			inSingleQuote bool
			inDoubleQuote bool
		)
		for {
			c, err := r.ReadByte()
			if errors.Is(err, io.EOF) {
				break
			}
			if c == '#' && !inSingleQuote && !inDoubleQuote {
				break
			}
			if c == '\\' && !isEscaped {
				isEscaped = true
				continue
			}
			if c == '\'' && !inDoubleQuote && !isEscaped {
				inSingleQuote = !inSingleQuote
				continue
			}
			if c == '"' && !inSingleQuote && !isEscaped {
				inDoubleQuote = !inDoubleQuote
				continue
			}
			isEscaped = false
			value += string(c)
		}
		env = append(env, fmt.Sprintf("%v=%v", strings.TrimSpace(key), strings.TrimSpace(value)))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}
