// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flakeTestData = `{"Time":"2024-06-14T22:24:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testname"}
{"Time":"2024-06-14T22:24:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"=== RUN   testname\n"}
{"Time":"2024-06-14T22:24:53.156271614Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"    file_test.go:10: flakytest: this is a known flaky test\n"}
{"Time":"2024-06-14T22:26:02.039003529Z","Action":"fail","Package":"a/b/c","Test":"testname","Elapsed":26.25}
`

const failedTestData = `{"Time":"2024-06-14T22:24:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testname"}
{"Time":"2024-06-14T22:24:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"=== RUN   testname\n"}
{"Time":"2024-06-14T22:26:02.039003529Z","Action":"fail","Package":"a/b/c","Test":"testname","Elapsed":26.25}
`

const rerunTestData = `{"Time":"2024-06-14T22:24:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testname"}
{"Time":"2024-06-14T22:24:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"=== RUN   testname\n"}
{"Time":"2024-06-14T22:26:02.039003529Z","Action":"fail","Package":"a/b/c","Test":"testname","Elapsed":26.25}
{"Time":"2024-06-14T22:27:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testname"}
{"Time":"2024-06-14T22:27:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testname","Output":"=== RUN   testname\n"}
{"Time":"2024-06-14T22:28:02.039003529Z","Action":"pass","Package":"a/b/c","Test":"testname","Elapsed":26.25}
`

const onlyParentOfFlakeFailed = `{"Time":"2024-06-14T22:24:52.156240262Z","Action":"run","Package":"a/b/c","Test":"testparent"}
{"Time":"2024-06-14T22:24:53.156240262Z","Action":"run","Package":"a/b/c","Test":"testparent/testname"}
{"Time":"2024-06-14T22:24:53.156263319Z","Action":"output","Package":"a/b/c","Test":"testparent/testname","Output":"=== RUN   testparent/testname\n"}
{"Time":"2024-06-14T22:24:53.156271614Z","Action":"output","Package":"a/b/c","Test":"testparent/testname","Output":"    file_test.go:10: flakytest: this is a known flaky test\n"}
{"Time":"2024-06-14T22:26:02.039003529Z","Action":"pass","Package":"a/b/c","Test":"testparent/testname","Elapsed":26.25}
{"Time":"2024-06-14T22:26:03.039003529Z","Action":"fail","Package":"a/b/c","Test":"testparent","Elapsed":28.25}
`

func TestFlakeInOutput(t *testing.T) {
	out, err := reviewTestsReaders(bytes.NewBuffer([]byte(flakeTestData)), nil)
	require.NoError(t, err)
	assert.Empty(t, out.Failed)
	assert.Equal(t, fmt.Sprintf(flakyFormat, "a/b/c", "testname"), out.Flaky)
	assert.Empty(t, out.ReRuns)
}

func TestFailedInOutput(t *testing.T) {
	out, err := reviewTestsReaders(bytes.NewBuffer([]byte(failedTestData)), nil)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(failFormat, "a/b/c", "testname"), out.Failed)
	assert.Empty(t, out.Flaky)
	assert.Empty(t, out.ReRuns)
}

func TestRerunInOutput(t *testing.T) {
	out, err := reviewTestsReaders(bytes.NewBuffer([]byte(rerunTestData)), nil)
	require.NoError(t, err)
	assert.Empty(t, out.Failed)
	assert.Empty(t, out.Flaky)
	assert.Equal(t, fmt.Sprintf(rerunFormat, "a/b/c", "testname", "pass"), out.ReRuns)
}

func TestOnlyParentOfFlakeFailed(t *testing.T) {
	out, err := reviewTestsReaders(bytes.NewBuffer([]byte(onlyParentOfFlakeFailed)), nil)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(failFormat, "a/b/c", "testparent"), out.Failed)
	assert.Empty(t, out.Flaky)
	assert.Empty(t, out.ReRuns)
}
