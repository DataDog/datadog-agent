// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package helpers

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertArchiveClosed(t *testing.T, path string) {
	t.Helper()
	// Windows cannot delete a file that is still open.
	assert.NoError(t, os.Remove(path), "the archive should be closed once the upload is over")
}
