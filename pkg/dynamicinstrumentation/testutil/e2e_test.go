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
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/kr/pretty"

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
	matches           []bool
	expectation       ditypes.CapturedValueMap
	unexpectedResults []ditypes.CapturedValueMap
}

var results = make(map[string]*testResult)

func TestGoDI(t *testing.T) {
	flake.Mark(t)
	if err := rlimit.RemoveMemlock(); err != nil {
		require.NoError(t, rlimit.RemoveMemlock())
	}

	if features.HaveMapType(ebpf.RingBuf) != nil {
		t.Skip("ringbuffers not supported on this kernel")
	}

	serviceName := "go-di-sample-service-" + randomLabel()
	sampleServicePath := BuildSampleService(t)
	cmd := exec.Command(sampleServicePath)
	cmd.Env = []string{
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		fmt.Sprintf("DD_SERVICE=%s", serviceName),
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
	for function, expectedCaptureTuples := range expectedCaptures {
		for _, expectedCaptureValue := range expectedCaptureTuples {
			// Generate config for this function
			buf = bytes.NewBuffer(b)
			functionWithoutPackagePrefix, _ := strings.CutPrefix(function, "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.")
			t.Log("Instrumenting ", functionWithoutPackagePrefix)
			results[function] = &testResult{
				testName:          functionWithoutPackagePrefix,
				expectation:       expectedCaptureValue.CapturedValueMap,
				matches:           []bool{},
				unexpectedResults: []ditypes.CapturedValueMap{},
			}
			err = cfgTemplate.Execute(buf, configDataType{
				ServiceName:  serviceName,
				FunctionName: functionWithoutPackagePrefix,
				CaptureDepth: expectedCaptureValue.Options.CaptureDepth,
			})
			require.NoError(t, err)
			eventOutputWriter.expectedResult = expectedCaptureValue.CapturedValueMap

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

	for i := range results {
		for _, ok := range results[i].matches {
			if !ok {
				t.Errorf("Failed test for: %s\nReceived event: %v\nExpected: %v",
					results[i].testName,
					pretty.Sprint(results[i].unexpectedResults),
					pretty.Sprint(results[i].expectation))
				break
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
	b, ok := results[funcName]
	if !ok {
		e.t.Errorf("received event from unexpected probe: %s", funcName)
		return
	}
	if !reflect.DeepEqual(e.expectedResult, actual) {
		b.matches = append(b.matches, false)
		b.unexpectedResults = append(b.unexpectedResults, actual)
		e.t.Error("received unexpected value")
	} else {
		b.matches = append(b.matches, true)
	}

	return len(p), nil
}

func scrubPointerValues(captures ditypes.CapturedValueMap) {
	for _, v := range captures {
		scrubPointerValue(v)
	}
}

func scrubPointerValue(capture *ditypes.CapturedValue) {
	if strings.HasPrefix(capture.Type, "*") {
		capture.Value = nil
	}
	scrubPointerValues(capture.Fields)
}

type configDataType struct {
	ServiceName  string
	FunctionName string
	CaptureDepth int
}

var configTemplateText = `
{
    "{{.ServiceName}}": {
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
                "maxReferenceDepth": {{.CaptureDepth}}
            },
            "sampling": {
                "snapshotsPerSecond": 5000
            },
            "evaluateAt": "EXIT"
        }
    }
}
`

func randomLabel() string {
	length := 6
	randomString := make([]byte, length)
	for i := 0; i < length; i++ {
		randomString[i] = byte(65 + rand.Intn(25))
	}
	return string(randomString)
}
