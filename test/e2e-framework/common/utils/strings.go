package utils

import "strings"

func IndentMultilineString(s string, indentation int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", indentation) + line
	}
	return strings.Join(lines, "\n")
}
