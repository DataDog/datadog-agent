// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// CIncludePattern is the regex for #include headers of C files
	CIncludePattern = `^\s*#include\s+"(.*)"$`
	includeRegexp   *regexp.Regexp
	ignoredHeaders  = map[string]struct{}{"vmlinux.h": {}}
)

func init() {
	includeRegexp = regexp.MustCompile(CIncludePattern)
}

// This program is intended to be called from go generate.
// It will preprocess a .c file to replace all the `#include "file.h"` statements with the header files contents
// while making sure to only include a file once.
// This does not process includes using angle brackets, e.g. `#include <stdio>`.
// You may optionally specify additional include directories to search.
func main() {
	if len(os.Args[1:]) < 2 {
		panic("please use 'go run include_headers.go <c_file> <output_file> [include_dir]...'")
	}

	args := os.Args[1:]
	inputFile, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("unable to get absolute path to %s: %s", args[0], err)
	}
	outputFile, err := filepath.Abs(args[1])
	if err != nil {
		log.Fatalf("unable to get absolute path to %s: %s", args[1], err)
	}

	err = runProcessing(inputFile, outputFile, args[2:])
	if err != nil {
		log.Fatalf("error including headers: %s", err)
	}
	fmt.Printf("successfully included headers from %s => %s\n", inputFile, outputFile)
}

func runProcessing(inputFile, outputFile string, dirs []string) error {
	var includeDirs []string
	for _, d := range dirs {
		dir, err := filepath.Abs(d)
		if err != nil {
			return fmt.Errorf("unable to get absolute path to %s: %s", d, err)
		}
		includeDirs = append(includeDirs, dir)
	}
	ps := newPathSearcher(includeDirs)

	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return err
	}

	of, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error opening output file: %s", err)
	}
	defer of.Close()

	if err := of.Chmod(0644); err != nil {
		return fmt.Errorf("error setting mode on output file: %s", err)
	}

	bof := bufio.NewWriter(of)

	includedFiles := make(map[string]struct{})
	if err := processIncludes(inputFile, bof, ps, includedFiles); err != nil {
		return fmt.Errorf("error processing includes: %s", err)
	}

	if err := bof.Flush(); err != nil {
		return fmt.Errorf("error flushing buffer to disk: %s", err)
	}

	if len(includedFiles) == 0 {
		return nil
	}

	root := rootDir(outputFile)
	depsFile := fmt.Sprintf("%s.d", outputFile)
	odeps, err := os.Create(depsFile)
	if err != nil {
		return fmt.Errorf("error opening output deps file: %s", err)
	}
	defer odeps.Close()

	relOut, err := filepath.Rel(root, outputFile)
	if err != nil {
		return fmt.Errorf("error getting relative path: %s", err)
	}
	odeps.WriteString(fmt.Sprintf("%s: \\\n", relOut))
	idx := 0
	for f := range includedFiles {
		rf, err := filepath.Rel(root, f)
		if err != nil {
			return fmt.Errorf("error getting relative path: %s", err)
		}
		odeps.WriteString(fmt.Sprintf("  %s", rf))
		if idx < (len(includedFiles) - 1) {
			odeps.WriteString(" \\")
		}
		odeps.WriteString("\n")
		idx++
	}
	return nil
}

func processIncludes(path string, out io.Writer, ps *pathSearcher, includedFiles map[string]struct{}) error {
	if _, included := includedFiles[path]; included {
		return nil
	}
	includedFiles[path] = struct{}{}
	log.Printf("included %s\n", path)

	sourceReader, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sourceReader.Close()

	scanner := bufio.NewScanner(sourceReader)
	for scanner.Scan() {
		match := includeRegexp.FindSubmatch(scanner.Bytes())
		if len(match) == 2 {
			headerName := string(match[1])
			if _, ok := ignoredHeaders[headerName]; ok {
				continue
			}
			headerPath, err := ps.findInclude(path, headerName)
			if err != nil {
				return fmt.Errorf("error searching for header: %s", err)
			}
			if err := processIncludes(headerPath, out, ps, includedFiles); err != nil {
				return err
			}
			continue
		}
		out.Write(scanner.Bytes())
		out.Write([]byte{'\n'})
	}
	return nil
}

type pathCacheEntry struct {
	srcPath    string
	headerName string
}

type pathSearcher struct {
	includeDirs []string
	cache       map[pathCacheEntry]string
}

func newPathSearcher(includeDirs []string) *pathSearcher {
	return &pathSearcher{
		includeDirs: includeDirs,
		cache:       make(map[pathCacheEntry]string),
	}
}

func isFilePresent(dir string, headerName string) (string, bool) {
	p := filepath.Join(dir, headerName)
	_, err := os.Stat(p)
	return p, err == nil
}

func (ps *pathSearcher) findIncludeInner(srcPath string, headerName string) (string, error) {
	if fullPath, ok := isFilePresent(filepath.Dir(srcPath), headerName); ok {
		return fullPath, nil
	}

	for _, dir := range ps.includeDirs {
		if fullPath, ok := isFilePresent(dir, headerName); ok {
			return fullPath, nil
		}
	}
	return "", fmt.Errorf("file %s not found", headerName)
}

func (ps *pathSearcher) findInclude(srcPath string, headerName string) (string, error) {
	ce := pathCacheEntry{
		srcPath:    srcPath,
		headerName: headerName,
	}

	if fullPath, present := ps.cache[ce]; present {
		return fullPath, nil
	}

	computed, err := ps.findIncludeInner(srcPath, headerName)
	if err == nil {
		ps.cache[ce] = computed
	}
	return computed, err
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
