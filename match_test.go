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
	"testing"
)

func TestMatch(t *testing.T) {
	type args struct {
		p    string
		path string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"*.go@/test/test.go", args{"*.go", "/test/test.go"}, true},
		{"*.ago@/test/test.go", args{"*.ago", "/test/test.go"}, false},
		{"test/*.go@/test/test.go", args{"test/*.go", "/test/test.go"}, true},
		{"test/*.ago@/test/test.go", args{"test/*.ago", "/test/test.go"}, false},
		{"**/test/*.go@/test/test.go", args{"**/test/*.go", "/test/test.go"}, true},
		{"**/test/*.ago@/test/test.go", args{"**/test/*.ago", "/test/test.go"}, false},
		{"**/test/aa/*.go@/test/test.go", args{"**/test/aa/*.go", "/test/test.go"}, false},
		{"**/test/aa/*.ago@/test/test.go", args{"**/test/aa/*.ago", "/test/test.go"}, false},
		{"**/test/**/test/**/*.go@/test/aa/test/test.go", args{"**/test/**/test/**/*.go", "/test/aa/test/test.go"}, true},
		{"**/test/**/test/**/*.go@/test/test.go", args{"**/test/**/test/**/*.go", "/test/test.go"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := match(tt.args.p, tt.args.path); got != tt.want {
				t.Errorf("match() = %v, want %v", got, tt.want)
			}
		})
	}
}
