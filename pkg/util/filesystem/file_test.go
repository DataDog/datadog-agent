// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadFileWithSizeLimit(t *testing.T) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatal(err)
	}
	filename := f.Name()

	_ = os.WriteFile(filename, []byte("test file"), 0644)

	// file can be read successfully if not too large
	data, err := ReadFileWithSizeLimit(filename, int64(10))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, data, []byte("test file"))

	// file will fail to be read if maxSize is too small
	_, err = ReadFileWithSizeLimit(filename, int64(5))
	assert.Error(t, err, "error expected")
	sizeLimitErr, ok := err.(*FileSizeLimitError)
	assert.True(t, ok)
	assert.Equal(t, sizeLimitErr.Size, int64(9))
	assert.Equal(t, sizeLimitErr.Limit, int64(5))

	// if file is not found, an os.PathError is returnedx
	filename = fmt.Sprintf("%sz", filename)
	_, err = ReadFileWithSizeLimit(filename, int64(10))
	assert.Error(t, err, "error expected")
	pathErr, ok := err.(*os.PathError)
	assert.True(t, ok)
	assert.Error(t, pathErr, "path error expected")
}
