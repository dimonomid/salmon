package setup

import (
	"strings"
	"text/template"
)

// RenderDesktopEntryTemplate renders a desktop entry template using
// desktop-entry-safe Exec arguments.
func RenderDesktopEntryTemplate(name, source string, data interface{}) (string, error) {
	return renderTemplate(name, source, data, template.FuncMap{
		"desktopExecArgument": desktopExecArgument,
	})
}

// desktopExecArgument formats an argument for a desktop entry Exec value.
func desktopExecArgument(argument string) string {
	var output strings.Builder
	output.WriteByte('"')
	for _, character := range argument {
		switch character {
		case '\\':
			// Desktop entry string decoding and Exec parsing each consume one
			// level of escaping.
			output.WriteString("\\\\\\\\")
		case '"', '`':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '$':
			output.WriteString("\\\\$")
		case '%':
			output.WriteString("%%")
		default:
			output.WriteRune(character)
		}
	}
	output.WriteByte('"')
	return output.String()
}
