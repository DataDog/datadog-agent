// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
	"github.com/stretchr/testify/assert"
)

// assertFilesExist verifies that all files in filenames exist in the flare archive
func assertFilesExist(t *testing.T, flare flare.Flare, filenames []string) {
	verifyAssertionsOnFilesList(t, flare, filenames, fileExists)
}

// fileExists verifies if a file exists in the flare archive
func fileExists(t *testing.T, flare flare.Flare, filename string) {
	_, err := flare.GetFile(filename)
	assert.NoError(t, err, "Got error when searching for '%v' file in flare archive: %v", filename, err)
}

// verifyAssetionsOnFilesList runs an assertion function on all files in filenames
func verifyAssertionsOnFilesList(t *testing.T, flare flare.Flare, filenames []string, assertFn func(*testing.T, flare.Flare, string)) {
	for _, filename := range filenames {
		assertFn(t, flare, filename)
	}
}
