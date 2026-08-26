package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DirPerm  = 0o750
	FilePerm = 0o600
)

// ValidateOutputPath ensures path is absolute, free of traversal, and under an allowed root when roots are set.
func ValidateOutputPath(path string, allowedRoots ...string) error {
	if path == "" {
		return fmt.Errorf("output path must not be empty")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("output path must be absolute")
	}
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("output path must not contain .. segments")
	}
	if len(allowedRoots) == 0 {
		return nil
	}
	for _, root := range allowedRoots {
		if root == "" {
			continue
		}
		rootClean := filepath.Clean(root)
		if pathWithin(clean, rootClean) {
			return nil
		}
	}
	return fmt.Errorf("output path %q is outside allowed workspace directories", clean)
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// SecureFile creates or truncates a file with restrictive permissions.
func SecureFile(path string, data []byte) error {
	return os.WriteFile(path, data, FilePerm)
}

// SecureMkdirAll creates a directory tree with DirPerm.
func SecureMkdirAll(path string) error {
	return os.MkdirAll(path, DirPerm)
}

// HardenExistingPath chmods an existing file or directory when owned by the current process.
func HardenExistingPath(path string, dir bool) error {
	perm := FilePerm
	if dir {
		perm = DirPerm
	}
	return os.Chmod(path, os.FileMode(perm))
}
