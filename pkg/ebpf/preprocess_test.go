// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata && linux_bpf

package ebpf

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreprocessFile(t *testing.T) {
	testBPFDir := t.TempDir()

	assetSource := `#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/oom.h>

#include "test-header.h"
`

	assetHeader := `#ifndef TEST_H
#define TEST_H

#include <linux/types.h>

#ifndef SOME_CONSTANT
#define SOME_CONSTANT 10
#endif

struct test_struct {
  __u32 id;
};

#endif /* defined(TEST_H) */
`

	if err := os.WriteFile(path.Join(testBPFDir, "test-asset.c"), []byte(assetSource), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path.Join(testBPFDir, "test-header.h"), []byte(assetHeader), 0644); err != nil {
		t.Fatal(err)
	}

	source, err := PreprocessFile(testBPFDir, "test-asset.c")

	sourceString := source.String()

	// Assert err is nil
	assert.Nil(t, err, err)

	// Assert negative examples source should not contain
	assert.NotContains(t, sourceString, "linux/sched.h")
	assert.NotContains(t, sourceString, "linux/stdio.h")

	// Assert examples of what source should contain
	assert.Contains(t, sourceString, "linux/oom.h")
	assert.Contains(t, sourceString, "linux/tcp.h")
	assert.Contains(t, sourceString, "linux/bpf.h")

	// Assert test-header.h content is present
	assert.Contains(t, sourceString, "TEST_H")
	assert.Contains(t, sourceString, "linux/types.h")
	assert.Contains(t, sourceString, "SOME_CONSTANT")
	assert.Contains(t, sourceString, "test_struct")
}
