// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFileRemovalPolicyUnknownDomain(t *testing.T) {
	a := assert.New(t)
	root := t.TempDir()
	p, err := NewFileRemovalPolicy(root, 1, FileRemovalPolicyTelemetry{})
	a.NoError(err)

	domain1, err := p.RegisterDomain("domain1")
	a.NoError(err)
	domain2, err := p.RegisterDomain("domain2")
	a.NoError(err)

	file1 := createRetryFile(a, domain1, "file1")
	file2 := createRetryFile(a, domain2, "file2")
	file3 := createRetryFile(a, path.Join(root, "unknownDomain"), "file3")
	file4 := createFile(a, path.Join(root, "unknownDomain"), "notRetryFileMustNotBeRemoved")

	pathsRemoved, err := p.RemoveUnknownDomains()
	a.NoError(err)
	assertFilenamesEqual(a, []string{file3}, pathsRemoved)
	assertFilenamesEqual(a, []string{file1, file2, file4}, getRemainingFiles(a, root))
}

func TestFileRemovalPolicyOutdatedFiles(t *testing.T) {
	a := assert.New(t)
	root := t.TempDir()
	outDatedFileDayCount := 2
	p, err := NewFileRemovalPolicy(root, outDatedFileDayCount, FileRemovalPolicyTelemetry{})
	a.NoError(err)

	domain, err := p.RegisterDomain("domain")
	a.NoError(err)

	file1 := createRetryFile(a, domain, "file1")
	file2 := createRetryFile(a, domain, "file2")
	file3 := createRetryFile(a, domain, "file3")

	modTime := time.Now().Add(time.Duration(-3*24) * time.Hour)
	a.NoError(os.Chtimes(file2, modTime, modTime))

	modTime = time.Now().Add(time.Duration(-1*24) * time.Hour)
	a.NoError(os.Chtimes(file3, modTime, modTime))

	pathsRemoved, err := p.RemoveOutdatedFiles()
	a.NoError(err)
	assertFilenamesEqual(a, []string{file2}, pathsRemoved)
	assertFilenamesEqual(a, []string{file1, file3}, getRemainingFiles(a, root))
}

func TestFileRemovalPolicyExistingDomain(t *testing.T) {
	a := assert.New(t)
	root := t.TempDir()
	telemetry := FileRemovalPolicyTelemetry{}
	_, err := NewFileRemovalPolicy(root, 1, telemetry)
	a.NoError(err)

	// No error if the folder already exits.
	_, err = NewFileRemovalPolicy(root, 1, telemetry)
	a.NoError(err)
}

func createRetryFile(a *assert.Assertions, root string, filename string) string {
	return createFile(a, root, filename+retryTransactionsExtension)
}

func createFile(a *assert.Assertions, root string, filename string) string {
	a.NoError(os.MkdirAll(root, 0755))
	fullPath := path.Join(root, filename)
	a.NoError(os.WriteFile(fullPath, []byte{1, 2, 3}, 0644))
	return fullPath
}

func getRemainingFiles(a *assert.Assertions, folder string) []string {
	var files []string
	a.NoError(filepath.Walk(folder,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				files = append(files, p)
			}
			return nil
		}))

	return files
}

func assertFilenamesEqual(a *assert.Assertions, expected []string, current []string) {
	a.EqualValues(getFilenames(expected), getFilenames(current))
}

func getFilenames(paths []string) []string {
	var filenames []string

	for _, p := range paths {
		filenames = append(filenames, filepath.Base(p))
	}
	return filenames
}
