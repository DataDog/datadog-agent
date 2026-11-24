// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package main

import (
	"bufio"
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	klogv2 "k8s.io/klog/v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func emitKlogErrorf() {
	klogv2.Errorf("klog errorf test")
}

func emitKlogErrorS() {
	klogv2.ErrorS(errors.New("boom"), "klog errors test")
}

func emitKlogErrorSDepth() {
	klogv2.ErrorSDepth(0, errors.New("boom"), "klog errorsdepth test")
}

func TestKlogRedirectCallerAttribution(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(l, log.DebugStr)

	emitKlogErrorf()
	emitKlogErrorS()
	emitKlogErrorSDepth()
	klogv2.Flush()
	require.NoError(t, w.Flush())

	out := b.String()
	assert.Contains(t, out, "[ERROR] emitKlogErrorf: klog errorf test")
	assert.Contains(t, out, "[ERROR] emitKlogErrorS: \"klog errors test\" err=\"boom\"")
	assert.Contains(t, out, "[ERROR] emitKlogErrorSDepth: \"klog errorsdepth test\" err=\"boom\"")
}
