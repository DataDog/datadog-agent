// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package detectors

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
)

func TestGoDetector(t *testing.T) {
	proc := &languagedetection.Process{Pid: int32(os.Getpid())}
	currentLanguageInfo, err := NewGoDetector().DetectLanguage(proc)
	require.NoError(t, err)

	assert.Equal(t, languagemodels.Go, currentLanguageInfo.Name)
	assert.Equal(t, runtime.Version(), currentLanguageInfo.Version)
}
