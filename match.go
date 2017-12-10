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
		t := strings.Replace(tmp, subp, "", -1)
		if t == tmp {
			return false
		}
		tmp = t
	}

	return true
}
