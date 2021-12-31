package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/mod/modfile"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println(`Usage: modparser <path> <prefix>

Reads a go.mod file in the provided path, and returns the list of requires from this go.mod file.
that are prefixed with the given prefix.

Example: modparser /go/src/github.com/DataDog/datadog-agent github.com/DataDog/datadog-agent`)
		os.Exit(1)
	}

	modPath := os.Args[1]
	if !strings.HasSuffix(modPath, "/") {
		modPath += "/"
	}

	prefix := os.Args[2]

	modFilename := modPath + "go.mod"

	data, err := os.ReadFile(modFilename)
	if err != nil {
		fmt.Printf("Couldn't read file %s\n", modFilename)
		os.Exit(1)
	}

	parsedFile, err := modfile.Parse(modFilename, data, nil)
	if err != nil {
		fmt.Printf("Couldn't parse mod file %s\n", modFilename)
		os.Exit(1)
	}

	for _, req := range parsedFile.Require {
		for _, token := range req.Syntax.Token {
			if strings.Contains(token, prefix) {
				fmt.Println(token)
			}
		}
	}
}
