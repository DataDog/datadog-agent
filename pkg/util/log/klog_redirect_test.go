// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKlogRedirectLoggerStackDepth(t *testing.T) {
	klogRedirectLogger := NewKlogRedirectLogger(2)
	assert.Equal(t, 2, klogRedirectLogger.stackDepth, "KlogRedirectLogger stack depth should match the provided value")
}

func TestKlogRedirectLoggerWrite(t *testing.T) {
	// create buffer to capture logs from the klog redirect logger
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, _ := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, DebugLvl)
	SetupLogger(l, DebugStr)

	klogRedirectLogger := NewKlogRedirectLogger(3)

	// first level of the string indicates the log level
	// Known levels are: I, W, E, F
	// X is unknown so should be logged as INFO
	klogRedirectLogger.Write([]byte("I0105 12:34:56.789012 threadid file:line] klog message"))
	klogRedirectLogger.Write([]byte("W0206 12:34:56.789012 threadid file:line] klog message"))
	klogRedirectLogger.Write([]byte("E0307 12:34:56.789012 threadid file:line] klog message"))
	klogRedirectLogger.Write([]byte("F0408 12:34:56.789012 threadid file:line] klog message"))
	klogRedirectLogger.Write([]byte("X0509 12:34:56.789012 threadid file:line] klog message"))

	w.Flush()

	assert.Subset(t, strings.Split(b.String(), "\n"), []string{
		"[INFO] tRunner: klog message",
		"[WARN] tRunner: klog message",
		"[ERROR] tRunner: klog message",
		"[CRITICAL] tRunner: klog message",
		"[INFO] tRunner: klog message",
	})
}

func TestStackDepthFromFunctionNames(t *testing.T) {
	tests := []struct {
		name     string
		fns      []string
		fallback int
		want     int
	}{
		{
			name: "skips klog v2 ErrorSDepth frames",
			fns: []string{
				"github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write",
				"k8s.io/klog/v2.(*loggingT).output",
				"k8s.io/klog/v2.(*loggingT).printS",
				"k8s.io/klog/v2.ErrorSDepth",
				"k8s.io/klog/v2.ErrorS",
				"k8s.io/client-go/tools/record.(*eventBroadcasterImpl).recordToSink",
			},
			fallback: 7,
			want:     6,
		},
		{
			name: "skips klog v2 and logr frames",
			fns: []string{
				"github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write",
				"k8s.io/klog/v2.(*loggingT).output",
				"k8s.io/klog/v2.(*klogger).Error",
				"github.com/go-logr/logr.Logger.Error",
				"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling.Sync",
			},
			fallback: 7,
			want:     5,
		},
		{
			name: "falls back when no klog frames are present",
			fns: []string{
				"github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write",
				"github.com/DataDog/datadog-agent/pkg/util/log.TestKlogRedirectLoggerWrite",
				"testing.tRunner",
			},
			fallback: 3,
			want:     3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stackDepthFromFunctionNames(tc.fns, tc.fallback))
		})
	}
}
