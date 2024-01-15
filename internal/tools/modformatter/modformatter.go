// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package independent-lint checks a go.mod file at a given path specified by the -path argument
// to ensure that it does not import the list of modules specified by the -deny argument. If
// the module is found, it exits with status code 1 and logs an error.
package main

import (
	"flag"
	"fmt"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	path       = flag.String("path", ".", "Path to go.mod file to format")
	formatFile = flag.Bool("formatFile", false, "Enable or Disable formatting of the mod file")
)

// Delete an element from a slice
func deleteElement(slice []string, index int) []string {
	return append(slice[:index], slice[index+1:]...)
}

// Append all dependency of a prefix (replace, required) into the lines of the formatted go.mod
func appendDeps(lines []string, deps []string, prefix string) []string {
	size := len(lines)
	if size < 1 {
		return lines
	} else if size < 2 {
		dep := fmt.Sprintf("%s %s", prefix, deps[0])
		lines = append(lines, dep)
	} else {
		dep := fmt.Sprintf("%s (", prefix)
		lines = append(lines, dep)
		for _, element := range deps {
			if len(element) < 1 {
				continue
			}
			prefixStr := ""
			if element[0] != '\t' {
				prefixStr = "\t"
			}
			dep := fmt.Sprintf("%s%s", prefixStr, element)
			lines = append(lines, dep)
		}
		lines = append(lines, ")\n")
	}

	return lines

}

// Clean the file from repetitive new lines
func removeExtraLines(lines []string, consecutiveLineNumber int) []string {
	lineCounter := 0
	indexList := make([]int, 0)
	for idx, content := range lines {
		if content == "" {
			lineCounter += 1
		} else {
			lineCounter = 0
		}
		if lineCounter > consecutiveLineNumber {
			indexList = append(indexList, idx)
		}
	}
	removedLinesCount := 0
	for _, index := range indexList {
		lines = deleteElement(lines, index-removedLinesCount)
		removedLinesCount += 1
	}
	return lines
}

// Format the content of a go.mod file
func formatModFile(content string) string {
	var requiredDeps []string
	var replaceDeps []string
	inReq := false
	inRep := false
	lines := strings.Split(content, "\n")
	totalRemovedLines := 0
	// Parse required and replaced dependencies
	for idx, line := range strings.Split(content, "\n") {
		if inReq {

			lines = deleteElement(lines, idx-totalRemovedLines)
			totalRemovedLines += 1
			if strings.Contains(line, ")") {
				inReq = false
			} else {
				requiredDeps = append(requiredDeps, line)
			}
		} else if inRep {
			lines = deleteElement(lines, idx-totalRemovedLines)
			totalRemovedLines += 1
			if strings.Contains(line, ")") {
				inRep = false
			} else {
				replaceDeps = append(replaceDeps, line)
			}
		} else if strings.HasPrefix(line, "replace") {
			parts := strings.Split(line, " ")
			lines = deleteElement(lines, idx-totalRemovedLines)
			totalRemovedLines += 1
			if parts[1][0] == '(' {
				inRep = true
			} else {
				replaceDeps = append(replaceDeps, strings.Join(parts[1:], " "))
			}
		} else if strings.HasPrefix(line, "require") {
			parts := strings.Split(line, " ")
			lines = deleteElement(lines, idx-totalRemovedLines)
			totalRemovedLines += 1
			if parts[1][0] == '(' {
				inReq = true
			} else {
				requiredDeps = append(requiredDeps, strings.Join(parts[1:], " "))
			}
		}
	}

	// Generate Formatted lines
	lines = appendDeps(lines, replaceDeps, "replace")
	lines = appendDeps(lines, requiredDeps, "require")

	// Remove leftover new lines
	lines = removeExtraLines(lines, 2)

	// join lines into string
	return strings.Join(lines, "\n")
}

func main() {
	flag.Parse()
	if *path == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Open and parse go.mod file
	modFilePath := filepath.Join(*path, "go.mod")
	b, err := os.ReadFile(modFilePath)
	if err != nil {
		fmt.Println("Error opening file %q:", err)
		return
	}
	f, err := modfile.Parse("go.mod", b, func(_, v string) (string, error) {
		return module.CanonicalVersion(v), nil
	})

	// Store every dependency already replaced in the go.mod file
	ReplaceMap := make(map[string]bool)
	for i := 0; i < len(f.Replace); i += 1 {
		token := f.Replace[i].Syntax.Token
		if len(token) < 1 {
			continue
		}
		if token[0] == "replace" {
			ReplaceMap[token[1]] = true
		} else {
			ReplaceMap[token[0]] = true
		}

	}

	// Check if every required internal dependency is replaced
	writeNeeded := false
	for _, req := range f.Require {
		if len(req.Syntax.Token) < 1 {
			continue
		}

		modname := req.Syntax.Token[0]
		if !strings.HasPrefix(modname, "github.com/DataDog/datadog-agent") {
			continue
		}

		if !ReplaceMap[modname] {
			modversion := req.Syntax.Token[1]
			fmt.Printf("Required %s %s is missing in replace of %s\n", modname, modversion, modFilePath)
			if *formatFile {
				writeNeeded = true
				trimName := strings.Split(modname, "github.com/DataDog/datadog-agent/")[1]
				trimPath := strings.Split(*path, "github.com/DataDog/datadog-agent/")[1]
				relativePath, err := filepath.Rel(trimPath, trimName)
				if err != nil {
					log.Fatal(err)
				}
				err = f.AddReplace(modname, "", relativePath, "")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("%s has been formatted !\n", modFilePath)
			}
		}
	}

	// Once all missing replace are added we're formatting the go.mod file.
	if writeNeeded {
		data, err := f.Format()
		if err != nil {
			log.Fatal(err)
		}
		formattedData := formatModFile(string(data))
		err = os.WriteFile(modFilePath, []byte(formattedData), 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

}
