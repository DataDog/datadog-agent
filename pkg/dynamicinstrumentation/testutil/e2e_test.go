// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"
)

func TestGoDI(t *testing.T) {
	sampleServicePath := BuildSampleService(t)
	cmd := exec.Command(sampleServicePath)
	cmd.Env = []string{
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_SERVICE=go-di-sample-service",
		"DD_DYNAMIC_INSTRUMENTATION_OFFLINE=true",
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		log.Println(cmd.Process.Kill())
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

	// Give DI time to start up
	time.Sleep(time.Second * 5)

	cm, ok := GoDI.ConfigManager.(*diconfig.ReaderConfigManager)
	if !ok {
		t.Error("Config manager is of wrong type")
	}

	cfgTemplate, err := template.New("config_template").Parse(configTemplateText)
	require.NoError(t, err)

	b := []byte{}
	var buf *bytes.Buffer
	for function, expectedCaptureValue := range expectedCaptures {
		// Generate config for this function
		buf = bytes.NewBuffer(b)
		functionWithoutPackagePrefix, _ := strings.CutPrefix(function, "main.")
		err = cfgTemplate.Execute(buf, configDataType{functionWithoutPackagePrefix})
		require.NoError(t, err)
		eventOutputWriter.doCompare = false
		eventOutputWriter.expectedResult = expectedCaptureValue

		// Read the configuration via the config manager
		_, err := cm.ConfigReader.Read(buf.Bytes())
		time.Sleep(time.Second * 5)
		if err != nil {
			t.Errorf("could not read new configuration: %s", err)
		}

		fmt.Printf("\n\n")
	}
}

type eventOutputTestWriter struct {
	t              *testing.T
	doCompare      bool
	expectedResult map[string]*ditypes.CapturedValue
}

func (e *eventOutputTestWriter) Write(p []byte) (n int, err error) {
	var snapshot ditypes.SnapshotUpload
	if err := json.Unmarshal(p, &snapshot); err != nil {
		e.t.Error("failed to unmarshal snapshot", err)
	}

	funcName := snapshot.Debugger.ProbeInSnapshot.Type + "." + snapshot.Debugger.ProbeInSnapshot.Method
	actual := snapshot.Debugger.Captures.Entry.Arguments
	scrubPointerValues(actual)
	if !reflect.DeepEqual(e.expectedResult, actual) {
		e.t.Error("Unexpected ", funcName)
		pretty.Log(actual)
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
                "typeName": "main",
                "methodName": "{{.FunctionName}}"
            },
            "tags": [],
            "template": "Executed main.{{.FunctionName}}, it took {@duration}ms",
            "segments": [
                {
                "str": "Executed main.{{.FunctionName}}, it took "
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
                "maxReferenceDepth": 3
            },
            "sampling": {
                "snapshotsPerSecond": 5000
            },
            "evaluateAt": "EXIT"
        }
    }
}
`
