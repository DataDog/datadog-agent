// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package config

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestExtractShortPathFromFullPath(t *testing.T) {
	// omnibus path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("/go/src/github.com/DataDog/datadog-agent/.omnibus/src/datadog-agent/src/github.com/DataDog/datadog-agent/pkg/collector/scheduler.go"))
	// dev env path
	assert.Equal(t, "cmd/agent/app/start.go", extractShortPathFromFullPath("/home/vagrant/go/src/github.com/DataDog/datadog-agent/cmd/agent/app/start.go"))
	// relative path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("pkg/collector/scheduler.go"))
	// no path
	assert.Equal(t, "main.go", extractShortPathFromFullPath("main.go"))
	// process agent
	assert.Equal(t, "cmd/agent/collector.go", extractShortPathFromFullPath("/home/jenkins/workspace/process-agent-build-ddagent/go/src/github.com/DataDog/datadog-process-agent/cmd/agent/collector.go"))
}

func benchmarkLogFormat(logFormat string, b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, logFormat)

	for n := 0; n < b.N; n++ {
		l.Infof("Hello I am a log")
	}
}

func BenchmarkLogFormatFilename(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%File:%Line in %FuncShort) | %Msg", b)
}

func BenchmarkLogFormatShortFilePath(b *testing.B) {
	benchmarkLogFormat("%Date(%s) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg", b)
}
