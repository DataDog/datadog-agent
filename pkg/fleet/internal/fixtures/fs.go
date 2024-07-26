// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fixtures contains test datadog package fixtures.
package fixtures

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertEqualFS asserts that two filesystems are equal.
func AssertEqualFS(t *testing.T, expected fs.FS, actual fs.FS) {
	t.Helper()
	err := fsContainsAll(expected, actual)
	assert.NoError(t, err)
	err = fsContainsAll(actual, expected)
	assert.NoError(t, err)
}

func fsContainsAll(a fs.FS, b fs.FS) error {
	return fs.WalkDir(a, ".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entryA, err := a.Open(path)
		if err != nil {
			return err
		}
		entryB, err := b.Open(path)
		if err != nil {
			return err
		}
		entryAStat, err := entryA.Stat()
		if err != nil {
			return err
		}
		entryBStat, err := entryB.Stat()
		if err != nil {
			return err
		}
		if entryAStat.IsDir() != entryBStat.IsDir() {
			return fmt.Errorf("files %s are not equal", path)
		}
		if entryAStat.IsDir() {
			return nil
		}
		contentA, err := io.ReadAll(entryA)
		if err != nil {
			return err
		}
		contentB, err := io.ReadAll(entryB)
		if err != nil {
			return err
		}
		if !bytes.Equal(contentA, contentB) {
			return fmt.Errorf("files %s do not have the same content: %s != %s", path, contentA, contentB)
		}
		return nil
	})
}
