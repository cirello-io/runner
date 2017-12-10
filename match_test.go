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
