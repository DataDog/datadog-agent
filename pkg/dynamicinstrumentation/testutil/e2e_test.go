// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diconfig"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/kr/pretty"

	"github.com/stretchr/testify/require"
)

func TestGoDI(t *testing.T) {
	flake.Mark(t)
	if err := rlimit.RemoveMemlock(); err != nil {
		require.NoError(t, rlimit.RemoveMemlock())
	}

	if features.HaveMapType(ebpf.RingBuf) != nil {
		t.Skip("ringbuffers not supported on this kernel")
	}

	sampleServicePath := BuildSampleService(t)
	cmd := exec.Command(sampleServicePath)
	cmd.Env = []string{
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_SERVICE=go-di-sample-service",
		"DD_DYNAMIC_INSTRUMENTATION_OFFLINE=true",
	}

	stdoutPipe, err1 := cmd.StdoutPipe()
	require.NoError(t, err1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			t.Log(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			t.Log(err)
		}
	}()

	// send stderr to stdout pipe
	cmd.Stderr = cmd.Stdout

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		t.Log(cmd.Process.Kill())
	})

	eventOutputWriter := &eventOutputTestWriter{
		t: t,
	}

	opts := &dynamicinstrumentation.DIOptions{
		RateLimitPerProbePerSecond: 0.0,
		ReaderWriterOptions: dynamicinstrumentation.ReaderWriterOptions{
			CustomReaderWriters: true,
			SnapshotWriter:      eventOutputWriter,
			DiagnosticWriter:    os.Stderr,
		},
	}

	var (
		GoDI *dynamicinstrumentation.GoDI
		err  error
	)

	GoDI, err = dynamicinstrumentation.RunDynamicInstrumentation(opts)
	require.NoError(t, err)
	t.Cleanup(GoDI.Close)

	cm, ok := GoDI.ConfigManager.(*diconfig.ReaderConfigManager)
	if !ok {
		t.Fatal("Config manager is of wrong type")
	}

	cfgTemplate, err := template.New("config_template").Parse(configTemplateText)
	require.NoError(t, err)

	b := []byte{}
	var buf *bytes.Buffer
	doCapture = false
	for function, expectedCaptureValue := range expectedCaptures {
		// Generate config for this function
		buf = bytes.NewBuffer(b)
		functionWithoutPackagePrefix, _ := strings.CutPrefix(function, "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.")
		t.Log("Instrumenting ", functionWithoutPackagePrefix)
		err = cfgTemplate.Execute(buf, configDataType{functionWithoutPackagePrefix})
		require.NoError(t, err)
		eventOutputWriter.doCompare = false
		eventOutputWriter.expectedResult = expectedCaptureValue

		// Read the configuration via the config manager
		_, err := cm.ConfigWriter.Write(buf.Bytes())
		time.Sleep(time.Second * 2)
		doCapture = true
		if err != nil {
			t.Errorf("could not read new configuration: %s", err)
		}
		time.Sleep(time.Second * 2)
		doCapture = false
	}
}

type eventOutputTestWriter struct {
	t              *testing.T
	doCompare      bool
	expectedResult map[string]*ditypes.CapturedValue
}

var doCapture bool

func (e *eventOutputTestWriter) Write(p []byte) (n int, err error) {
	if !doCapture {
		return 0, nil
	}
	var snapshot ditypes.SnapshotUpload
	if err := json.Unmarshal(p, &snapshot); err != nil {
		e.t.Error("failed to unmarshal snapshot", err)
	}

	funcName := snapshot.Debugger.ProbeInSnapshot.Type + "." + snapshot.Debugger.ProbeInSnapshot.Method
	actual := snapshot.Debugger.Captures.Entry.Arguments
	scrubPointerValues(actual)
	if !reflect.DeepEqual(e.expectedResult, actual) {
		e.t.Error("Unexpected ", funcName, pretty.Sprint(actual))
		e.t.Log("Expected: ", pretty.Sprint(e.expectedResult))
	}

	return len(p), nil
}

func scrubPointerValues(captures ditypes.CapturedValueMap) {
	for _, v := range captures {
		scrubPointerValue(v)
	}
}

func scrubPointerValue(capture *ditypes.CapturedValue) {
	if capture.Type == "ptr" {
		capture.Value = nil
	}
	scrubPointerValues(capture.Fields)
}

type configDataType struct{ FunctionName string }

var configTemplateText = `
{
    "go-di-sample-service": {
        "e504163d-f367-4522-8905-fe8bc34eb975": {
            "id": "e504163d-f367-4522-8905-fe8bc34eb975",
            "version": 0,
            "type": "LOG_PROBE",
            "language": "go",
            "where": {
                "typeName": "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample",
                "methodName": "{{.FunctionName}}"
            },
            "tags": [],
            "template": "Executed github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.{{.FunctionName}}, it took {@duration}ms",
            "segments": [
                {
                "str": "Executed github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.{{.FunctionName}}, it took "
                },
                {
                "dsl": "@duration",
                "json": {
                    "ref": "@duration"
                }
                },
                {
                "str": "ms"
                }
            ],
            "captureSnapshot": false,
            "capture": {
                "maxReferenceDepth": 6
            },
            "sampling": {
                "snapshotsPerSecond": 5000
            },
            "evaluateAt": "EXIT"
        }
    }
}
`
