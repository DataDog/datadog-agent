// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"debug/elf"
	"debug/gosym"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	codeowners "github.com/hairyhenderson/go-codeowners"
)

// testowners is a struct that holds the owners of the code. Not a full implementation, just
// enough to show owners in KMT tests
type testowners struct {
	owners        *codeowners.Codeowners
	testRootDir   string
	symtableCache map[string]*gosym.Table
}

func newTestowners(path string, testRootDir string) (*testowners, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %s", path, err)
	}
	defer f.Close()

	return newTestownersWithReader(f, testRootDir)
}

func newTestownersWithReader(contents io.Reader, testRootDir string) (*testowners, error) {
	owners, err := codeowners.FromReader(contents, "")
	if err != nil {
		return nil, fmt.Errorf("parse codeowners: %s", err)
	}

	return &testowners{owners: owners, testRootDir: testRootDir, symtableCache: make(map[string]*gosym.Table)}, nil
}

func (c *testowners) loadSymbolMapForFile(file string) error {
	if _, ok := c.symtableCache[file]; ok {
		return nil
	}
	c.symtableCache[file] = nil // Initialize to nil to avoid loading multiple times even if an error occurs

	elfFile, err := elf.Open(file)
	if err != nil {
		return fmt.Errorf("open %s: %s", file, err)
	}
	defer elfFile.Close()

	lineTableSect := elfFile.Section(".gopclntab")
	textSect := elfFile.Section(".text")
	if lineTableSect == nil || textSect == nil {
		return fmt.Errorf("missing required sections in %s", file)
	}

	lineTableData, err := lineTableSect.Data()
	if err != nil {
		return fmt.Errorf("read .gopclntab: %s", err)
	}

	textAddr := elfFile.Section(".text").Addr
	lineTable := gosym.NewLineTable(lineTableData, textAddr)
	symTable, err := gosym.NewTable([]byte{}, lineTable)
	if err != nil {
		return fmt.Errorf("parse .gosymtab: %s", err)
	}

	c.symtableCache[file] = symTable

	return nil
}

func (c *testowners) getFileForTest(ev testEvent, testBinary string) string {
	if testBinary == "" {
		if c.testRootDir == "" {
			return ""
		}

		testBinary = filepath.Join(c.testRootDir, ev.Package, "testsuite")
	}

	err := c.loadSymbolMapForFile(testBinary)
	if err != nil {
		return ""
	}

	symTable := c.symtableCache[testBinary]
	if symTable == nil {
		return ""
	}

	testName := strings.Split(ev.Test, "/")[0]

	var testFunc *gosym.Func
	// Test name to function name is not a 1:1 mapping, so we need to iterate over all functions to find the right one
	for i := range symTable.Funcs {
		f := &symTable.Funcs[i]
		// First, check if the function name contains the package name
		if !strings.Contains(f.Name, ev.Package) {
			continue
		}

		// Now, get the function name (the last part of the function name separated by dots)
		parts := strings.Split(f.Name, ".")
		funcName := parts[len(parts)-1]
		if funcName == testName {
			testFunc = f
			break
		}
	}

	if testFunc == nil {
		return ""
	}

	funcFile, _, _ := symTable.PCToLine(testFunc.Entry)
	if funcFile == "" {
		return ""
	}

	// Now return only the path inside the repo, using the package prefix
	packageStart := strings.Index(funcFile, ev.Package)
	if packageStart == -1 {
		return "" // Should not happen as we already checked that the function name contains the package name, but just in case
	}

	return funcFile[packageStart:]
}

// matchTest returns the owners of the test file
func (c *testowners) matchTest(ev testEvent) string {
	testFile := c.getFileForTest(ev, "")
	if testFile == "" {
		testFile = ev.Package // Fallback to package name
	}

	return strings.Join(c.owners.Owners(testFile), " ")
}

// matchTestWithBinary returns the owners of the test file, overriding the automatic detection of the test binary file
func (c *testowners) matchTestWithBinary(ev testEvent, binaryFile string) string {
	testFile := c.getFileForTest(ev, binaryFile)
	if testFile == "" {
		testFile = ev.Package // Fallback to package name
	}

	return strings.Join(c.owners.Owners(testFile), " ")
}
