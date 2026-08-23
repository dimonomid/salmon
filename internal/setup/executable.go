package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// ExecutablePath returns this executable's resolved path, rejecting a
// temporary path that would not survive installation.
func ExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	tempDir := filepath.Clean(os.TempDir())
	if path == tempDir || len(path) > len(tempDir) && path[:len(tempDir)+1] == tempDir+string(os.PathSeparator) {
		return "", fmt.Errorf("refusing to install from temporary executable %s", path)
	}
	return path, nil
}
