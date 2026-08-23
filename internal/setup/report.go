package setup

import (
	"fmt"
	"io"
	"strings"
)

// ReportEnsureResult reports whether a user-managed file was created or an
// existing one was preserved.
func ReportEnsureResult(output io.Writer, description, path string, created bool) error {
	if created {
		_, err := fmt.Fprintf(output, "Created %s at %s\n", description, path)
		return err
	}
	_, err := fmt.Fprintf(output, "%s already exists at %s; leaving it unchanged\n", strings.ToUpper(description[:1])+description[1:], path)
	return err
}
