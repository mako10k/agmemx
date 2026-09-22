package domain

import (
	"path/filepath"
	"strings"
)

// InSubtree reports whether domain is root or a directory inside root.
func InSubtree(root, domain string) bool {
	if domain == root {
		return true
	}
	return strings.HasPrefix(domain, root+string(filepath.Separator))
}
