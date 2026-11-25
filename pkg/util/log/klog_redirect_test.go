// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"runtime"
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

func TestParseKlogHeaderCaller(t *testing.T) {
	file, line, ok := parseKlogHeaderCaller("E0307 12:34:56.789012 12345 event.go:42")
	assert.True(t, ok)
	assert.Equal(t, "event.go", file)
	assert.Equal(t, 42, line)

	_, _, ok = parseKlogHeaderCaller("E0307 12:34:56.789012 threadid file:line")
	assert.False(t, ok)

	_, _, ok = parseKlogHeaderCaller("malformed")
	assert.False(t, ok)
}

func TestResolveKlogFrameDepth(t *testing.T) {
	tests := []struct {
		name            string
		frames          []runtime.Frame
		headerFile      string
		headerLine      int
		hasHeaderCaller bool
		fallback        int
		want            int
		wantReads       int
	}{
		{
			name: "skips klog v2 frames and stops immediately without header caller",
			frames: []runtime.Frame{
				{Function: "github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write"},
				{Function: "k8s.io/klog/v2.(*loggingT).output"},
				{Function: "k8s.io/klog/v2.(*loggingT).printS"},
				{Function: "k8s.io/klog/v2.ErrorSDepth"},
				{Function: "k8s.io/klog/v2.ErrorS"},
				{Function: "k8s.io/client-go/tools/record.(*eventBroadcasterImpl).recordToSink", File: "/go/pkg/mod/k8s.io/client-go/tools/record/event.go", Line: 100},
				{Function: "should.not.be.read"},
			},
			fallback:  7,
			want:      6,
			wantReads: 6,
		},
		{
			name: "honors nonzero ErrorSDepth via header file:line and stops once matched",
			frames: []runtime.Frame{
				{Function: "github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write"},
				{Function: "k8s.io/klog/v2.(*loggingT).output"},
				{Function: "k8s.io/klog/v2.ErrorSDepth"},
				{Function: "example.com/pkg.logWrapper", File: "/src/pkg/wrapper.go", Line: 12},
				{Function: "example.com/pkg.realCaller", File: "/src/pkg/caller.go", Line: 88},
				{Function: "should.not.be.read"},
			},
			headerFile:      "caller.go",
			headerLine:      88,
			hasHeaderCaller: true,
			fallback:        7,
			want:            5,
			wantReads:       5,
		},
		{
			name: "skips klog v2 and logr frames",
			frames: []runtime.Frame{
				{Function: "github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write"},
				{Function: "k8s.io/klog/v2.(*loggingT).output"},
				{Function: "k8s.io/klog/v2.(*klogger).Error"},
				{Function: "github.com/go-logr/logr.Logger.Error"},
				{Function: "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling.Sync", File: "/src/pkg/clusteragent/autoscaling/sync.go", Line: 55},
			},
			headerFile:      "sync.go",
			headerLine:      55,
			hasHeaderCaller: true,
			fallback:        7,
			want:            5,
			wantReads:       5,
		},
		{
			name: "falls back when no klog frames are present",
			frames: []runtime.Frame{
				{Function: "github.com/DataDog/datadog-agent/pkg/util/log.KlogRedirectLogger.Write"},
				{Function: "github.com/DataDog/datadog-agent/pkg/util/log.TestKlogRedirectLoggerWrite"},
				{Function: "testing.tRunner"},
			},
			fallback:  3,
			want:      3,
			wantReads: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := 0
			next := func() (runtime.Frame, bool) {
				f := tc.frames[idx]
				idx++
				return f, idx < len(tc.frames)
			}
			got := resolveKlogFrameDepth(next, tc.headerFile, tc.headerLine, tc.hasHeaderCaller, tc.fallback)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantReads, idx)
		})
	}
}
