// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
package tests

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"text/template"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type simpleTest struct {
	root string
}

func newSimpleTest(tb testing.TB, macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, testDir string) (*simpleTest, error) {
	t := &simpleTest{
		root: testDir,
	}

	if testDir == "" {
		dir, err := createTempDir(tb)
		if err != nil {
			return nil, err
		}
		t.root = dir
	}

	if err := t.load(macros, rules); err != nil {
		return nil, err
	}

	return t, nil
}
func (t *simpleTest) Root() string {
	return t.root
}

func (t *simpleTest) ProcessName() string {
	executable, _ := os.Executable()
	return path.Base(executable)
}

func (t *simpleTest) Path(filename ...string) (string, unsafe.Pointer, error) {
	components := []string{t.root}
	components = append(components, filename...)
	path := path.Join(components...)
	filenamePtr, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", nil, err
	}
	return path, unsafe.Pointer(filenamePtr), nil
}

func (t *simpleTest) load(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition) (err error) {
	executeExpressionTemplate := func(expression string) (string, error) {
		buffer := new(bytes.Buffer)
		tmpl, err := template.New("").Parse(expression)
		if err != nil {
			return "", err
		}

		if err := tmpl.Execute(buffer, t); err != nil {
			return "", err
		}

		return buffer.String(), nil
	}

	for _, rule := range rules {
		if rule.Expression, err = executeExpressionTemplate(rule.Expression); err != nil {
			return err
		}
	}

	for _, macro := range macros {
		if macro.Expression, err = executeExpressionTemplate(macro.Expression); err != nil {
			return err
		}
	}

	return nil
}

func createTempDir(tb testing.TB) (string, error) {
	dir := tb.TempDir()
	targetFileMode := fs.FileMode(0o711)

	// chmod the root and its parent since TempDir returns a 2-layers directory `/tmp/TestNameXXXX/NNN/`
	if err := os.Chmod(dir, targetFileMode); err != nil {
		return "", err
	}
	if err := os.Chmod(filepath.Dir(dir), targetFileMode); err != nil {
		return "", err
	}

	return dir, nil
}
