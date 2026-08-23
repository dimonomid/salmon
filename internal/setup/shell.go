package setup

import "strings"

// ShellArgument formats an argument for a POSIX shell command line. It leaves
// ordinary path-like values unquoted to keep command hints easy to read.
func ShellArgument(argument string) string {
	if argument != "" && strings.IndexFunc(argument, func(character rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", character)
	}) == -1 {
		return argument
	}
	return "'" + strings.ReplaceAll(argument, "'", "'\"'\"'") + "'"
}
