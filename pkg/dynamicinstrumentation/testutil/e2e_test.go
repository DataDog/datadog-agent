// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/kr/pretty"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diconfig"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	consumerstestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

	"github.com/stretchr/testify/require"
)

func TestGoDI(t *testing.T) {
	flake.Mark(t)
	require.NoError(t, rlimit.RemoveMemlock())
	if features.HaveMapType(ebpf.RingBuf) != nil {
		t.Skip("ringbuffers not supported on this kernel")
	}

	for function, expectedCaptureTuples := range expectedCaptures {
		for _, expectedCaptureValue := range expectedCaptureTuples {
			justFunctionName := string(function[strings.LastIndex(function, ".")+1:])
			t.Run(justFunctionName, func(t *testing.T) {
				runTestCase(t, function, expectedCaptureValue)
			})
		}
	}
}

func runTestCase(t *testing.T, function string, expectedCaptureValue CapturedValueMapWithOptions) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serviceName := "go-di-sample-service-" + randomLabel()
	sampleServicePath := BuildSampleService(t)
	cmd := exec.CommandContext(ctx, sampleServicePath)
	cmd.WaitDelay = 10 * time.Millisecond
	cmd.Env = []string{
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		fmt.Sprintf("DD_SERVICE=%s", serviceName),
		"DD_DYNAMIC_INSTRUMENTATION_OFFLINE=true",
	}

	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			t.Log("sample_service:", scanner.Text())
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Log("sample_service:", err)
		}
	}()
	t.Cleanup(func() {
		stdoutPipe.Close()
	})

	// send stderr to stdout pipe
	cmd.Stderr = cmd.Stdout

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		t.Logf("stopping %d", cmd.Process.Pid)
		_ = cmd.Process.Signal(os.Interrupt)
		_ = cmd.Wait()
	})
	t.Logf("launched %s as %d", serviceName, cmd.Process.Pid)

	eventOutputWriter := &eventOutputTestWriter{
		snapshots: make(chan ditypes.SnapshotUpload, 100),
	}
	t.Cleanup(func() { close(eventOutputWriter.snapshots) })

	diagnostics.Diagnostics = diagnostics.NewDiagnosticManager()
	t.Cleanup(diagnostics.StopGlobalDiagnostics)

	opts := &dynamicinstrumentation.DIOptions{
		RateLimitPerProbePerSecond: 0.0,
		ReaderWriterOptions: dynamicinstrumentation.ReaderWriterOptions{
			CustomReaderWriters: true,
			SnapshotWriter:      eventOutputWriter,
			DiagnosticWriter:    os.Stderr,
		},
	}

	GoDI, err := dynamicinstrumentation.RunDynamicInstrumentation(ctx, consumerstestutil.NewTestProcessConsumer(t), opts)
	require.NoError(t, err)
	t.Cleanup(GoDI.Close)

	cm, ok := GoDI.ConfigManager.(*diconfig.ReaderConfigManager)
	require.True(t, ok, "Config manager is of wrong type")

	cfgTemplate, err := template.New("config_template").Parse(configTemplateText)
	require.NoError(t, err, "template parse")

	// Generate config for this function
	functionWithoutPackagePrefix := strings.TrimPrefix(function, "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample.")
	t.Log("Instrumenting", functionWithoutPackagePrefix)
	buf := &bytes.Buffer{}
	err = cfgTemplate.Execute(buf, configDataType{
		ServiceName:  serviceName,
		FunctionName: functionWithoutPackagePrefix,
		CaptureDepth: expectedCaptureValue.Options.CaptureDepth,
	})
	require.NoError(t, err, "template execute")

	cm.ProcTracker.HandleProcessStartSync(uint32(cmd.Process.Pid))

	// Read the configuration via the config manager
	_ = cm.ConfigWriter.WriteSync(buf.Bytes())

	var lastSnapshot ditypes.CapturedValueMap
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		snapshot, ok := <-eventOutputWriter.snapshots
		if !ok {
			return
		}
		actual := snapshot.Debugger.Captures.Entry.Arguments
		scrubPointerValues(actual)
		compareCapturedValues(t, "", expectedCaptureValue.CapturedValueMap, actual)
		lastSnapshot = actual
	}, 5*time.Second, 100*time.Millisecond)

	if t.Failed() && lastSnapshot != nil {
		t.Logf("Expected:\n%s", pretty.Sprint(expectedCaptureValue.CapturedValueMap))
		t.Logf("Actual:\n%s", pretty.Sprint(lastSnapshot))
	}
}

type eventOutputTestWriter struct {
	snapshots chan ditypes.SnapshotUpload
}

// compareCapturedValues compares two CapturedValueMap objects in a deterministic way.
// This function is needed because the test results are stored in maps, which don't guarantee
// a consistent iteration order.
//
// The function ensures consistent comparison by:
// 1. Comparing map lengths first
// 2. Sorting keys before comparison
// 3. Recursively comparing nested fields
// 4. Comparing all relevant fields (Type, NotCapturedReason, Value) in a deterministic order
func compareCapturedValues(t assert.TestingT, path string, expected, actual ditypes.CapturedValueMap) {
	expectedKeys := slices.Collect(maps.Keys(expected))
	actualKeys := slices.Collect(maps.Keys(actual))
	assert.ElementsMatch(t, expectedKeys, actualKeys, "map keys")

	for _, k := range expectedKeys {
		expectedVal, eok := expected[k]
		actualVal, aok := actual[k]
		if !eok || !aok {
			continue
		}

		var prefix string
		if path != "" {
			prefix = fmt.Sprintf("%s.%s", path, k)
		} else {
			prefix = k
		}
		compareCapturedValue(t, prefix, expectedVal, actualVal)
	}
}

func compareCapturedValue(t assert.TestingT, path string, expected, actual *ditypes.CapturedValue) {
	assert.Equal(t, expected.Type, actual.Type, "Path: %q\nField: Type", path)
	assert.Equal(t, stringComparePtrValue(expected.Value), stringComparePtrValue(actual.Value), "Path: %q\nField: Value", path)
	compareCapturedValues(t, path, expected.Fields, actual.Fields)
	// Entries seems unused, so don't compare
	assert.Len(t, actual.Elements, len(expected.Elements), "Path: %q\nField: Elements (length)", path)
	for i := range expected.Elements {
		if i >= len(actual.Elements) {
			continue
		}
		compareCapturedValue(t, fmt.Sprintf("%s.Elements[%d]", path, i), &expected.Elements[i], &actual.Elements[i])
	}

	assert.Equal(t, expected.NotCapturedReason, actual.NotCapturedReason, "Path: %q\nField: NotCapturedReason", path)
	assert.Equal(t, expected.IsNull, actual.IsNull, "Path: %q\nField: IsNull", path)
	assert.Equal(t, expected.Size, actual.Size, "Path: %q\nField: Size", path)
	assert.Equal(t, expected.Truncated, actual.Truncated, "Path: %q\nField: Truncated", path)
}

func stringComparePtrValue(x *string) any {
	if x == nil {
		return nil
	}
	return *x
}

func (e *eventOutputTestWriter) Write(p []byte) (n int, err error) {
	var snapshot ditypes.SnapshotUpload
	err = json.Unmarshal(p, &snapshot)
	if err != nil {
		return 0, err
	}

	e.snapshots <- snapshot
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
