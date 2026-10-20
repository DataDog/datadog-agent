// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package rm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// On Unix, "\" is a valid filename character, not a path separator. rm must
// treat a trailing "\" as literal filename text, not directory syntax
// requiring the target to resolve as a directory.
func TestRmBackslashFilenameIsLiteralOnUnix(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `foo\`, "hello")
	_, stderr, code := rmRun(t, `rm 'foo\'`, dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assertGone(t, path)
}
