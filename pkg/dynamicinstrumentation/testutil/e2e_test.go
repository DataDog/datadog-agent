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

	"github.com/stretchr/testify/require"
)

type testResult struct {
	testName          string
	successTally      []bool
	expectation       ditypes.CapturedValueMap
	unexpectedResults []ditypes.CapturedValueMap
}

var eventsTally = make(map[string]*testResult)

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
		eventsTally[function] = &testResult{
			testName:          functionWithoutPackagePrefix,
			expectation:       expectedCaptureValue,
			successTally:      []bool{},
			unexpectedResults: []ditypes.CapturedValueMap{},
		}
		err = cfgTemplate.Execute(buf, configDataType{functionWithoutPackagePrefix})
		require.NoError(t, err)
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

probeLoop:
	for i := range eventsTally {
		for _, ok := range eventsTally[i].successTally {
			if !ok {
				t.Errorf("Failed test for: %s\nReceived event: %v\nExpected: %v",
					eventsTally[i].testName,
					eventsTally[i].unexpectedResults,
					eventsTally[i].expectation)
				continue probeLoop
			}
		}
	}
}

type eventOutputTestWriter struct {
	t              *testing.T
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

	funcName := snapshot.Debugger.ProbeInSnapshot.Method
	actual := snapshot.Debugger.Captures.Entry.Arguments
	scrubPointerValues(actual)
	b, ok := eventsTally[funcName]
	if !ok {
		e.t.Errorf("received event from unexpected probe: %s", funcName)
		return
	}
	if !reflect.DeepEqual(e.expectedResult, actual) {
		b.successTally = append(b.successTally, false)
		b.unexpectedResults = append(b.unexpectedResults, actual)
		e.t.Error("received unexpected value")
	} else {
		b.successTally = append(b.successTally, true)
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
                "maxReferenceDepth": 10
            },
            "sampling": {
                "snapshotsPerSecond": 5000
            },
            "evaluateAt": "EXIT"
        }
    }
}
`
