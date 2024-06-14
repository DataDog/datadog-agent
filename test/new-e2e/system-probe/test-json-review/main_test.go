// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flakeTestData = `{"Time":"2024-06-14T22:24:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testname"}
{"Time":"2024-06-14T22:24:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"=== RUN   testname\n"}
{"Time":"2024-06-14T22:24:53.156271614Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"    file_test.go:10: flakytest: this is a known flaky test\n"}
{"Time":"2024-06-14T22:26:02.039003529Z","Action":"fail","Package":"a/b/c","Test":"testname","Elapsed":26.25}
`

func TestFlakeInOutput(t *testing.T) {
	out, err := reviewTestsReaders(bytes.NewBuffer([]byte(flakeTestData)), nil)
	require.NoError(t, err)
	assert.Empty(t, out.Failed)
	assert.NotEmpty(t, out.Flaky)
	assert.Empty(t, out.ReRuns)
}
