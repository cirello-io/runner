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

package runner

import (
	"path/filepath"
	"strings"
)

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
