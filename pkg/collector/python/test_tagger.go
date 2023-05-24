// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

/*
#include <stdlib.h>
#include <string.h>

int arraylen(char **array, int max_len) {
	int i;
	for (i = 0; array[i]; i++){
		if (i == max_len) {
			return -1;
		}
	}
	return i;
}

*/
import "C"

func tagsMock(string, collectors.TagCardinality) ([]string, error) {
	return []string{"tag1", "tag2", "tag3"}, nil
}

func tagsMockNull(string, collectors.TagCardinality) ([]string, error) {
	return nil, nil
}

func tagsMockEmpty(string, collectors.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func testTags(t *testing.T) {
	tagsFunc = tagsMock
	defer func() { tagsFunc = tagger.Tag }()

	id := C.CString("test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.NotNil(t, res)
	require.Equal(t, C.arraylen(res, 3), C.int(3))

	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(unsafe.Pointer(res))[:4:4] // hardcoded expected length
	assert.Equal(t, C.GoString(indexTag[0]), "tag1")
	assert.Equal(t, C.GoString(indexTag[1]), "tag2")
	assert.Equal(t, C.GoString(indexTag[2]), "tag3")
	assert.Equal(t, unsafe.Pointer(indexTag[3]), unsafe.Pointer(nil))
}

func testTagsNull(t *testing.T) {
	tagsFunc = tagsMockNull
	defer func() { tagsFunc = tagger.Tag }()

	id := C.CString("test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.Nil(t, res)
}

func testTagsEmpty(t *testing.T) {
	tagsFunc = tagsMockEmpty
	defer func() { tagsFunc = tagger.Tag }()

	id := C.CString("test")
	defer C.free(unsafe.Pointer(id))

	res := Tags(id, 0)
	require.Nil(t, res)
}
