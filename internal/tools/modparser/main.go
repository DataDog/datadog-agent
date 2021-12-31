package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/mod/modfile"
)

func parseMod(module string) (*modfile.File, error) {
	if !strings.HasSuffix(module, "/") {
		module += "/"
	}

	modFilename := module + "go.mod"

	data, err := os.ReadFile(modFilename)
	if err != nil {
		return nil, fmt.Errorf("could not read go.mod file in %s", module)
	}

	parsedFile, err := modfile.Parse(modFilename, data, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse go.mod file in %s", module)
	}

	return parsedFile, nil
}

func filter(file *modfile.File, filter string) []string {
	var matches []string
	for _, req := range file.Require {
		for _, token := range req.Syntax.Token {
			if strings.HasPrefix(token, filter) {
				matches = append(matches, token)
			}
		}
	}
	return matches
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println(`Usage: modparser <path> <prefix>

Reads a go.mod file in the provided path, and prints the list of requires from this go.mod file.
that are prefixed with the given prefix.

Example: modparser /go/src/github.com/DataDog/datadog-agent github.com/DataDog/datadog-agent`)
		os.Exit(1)
	}

	modPath := os.Args[1]
	prefix := os.Args[2]

	parsedFile, err := parseMod(modPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, match := range filter(parsedFile, prefix) {
		fmt.Println(match)
	}
}
