package setup

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// EnsureFile atomically creates path with contents when it does not already
// exist. It never overwrites an existing user-managed file.
func EnsureFile(path, contents string) (bool, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return false, fmt.Errorf("create parent directory: %w", err)
	}

	file, err := ioutil.TempFile(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return false, fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := file.Name()
	closeAttempted := false
	defer func() {
		if !closeAttempted {
			file.Close()
		}
		os.Remove(temporaryPath)
	}()

	if err := file.Chmod(0644); err != nil {
		return false, fmt.Errorf("set file permissions: %w", err)
	}
	if _, err := file.WriteString(contents); err != nil {
		return false, fmt.Errorf("write file: %w", err)
	}
	closeAttempted = true
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close file: %w", err)
	}
	if err := os.Link(temporaryPath, path); os.IsExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("create file: %w", err)
	}
	return true, nil
}
