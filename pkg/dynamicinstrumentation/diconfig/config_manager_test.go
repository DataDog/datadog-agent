// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

func TestBackOff(t *testing.T) {

	compilerErrorProbe := &ditypes.Probe{
		ID:       "abc123",
		FuncName: "compilerError",
		InstrumentationInfo: &ditypes.InstrumentationInfo{
			InstrumentationOptions: &ditypes.InstrumentationOptions{
				CaptureParameters: true,
				MaxReferenceDepth: 5,
			},
		},
	}

	verifierErrorProbe := &ditypes.Probe{
		ID:       "xyz789",
		FuncName: "verifierError",
		InstrumentationInfo: &ditypes.InstrumentationInfo{
			InstrumentationOptions: &ditypes.InstrumentationOptions{
				CaptureParameters: true,
				MaxReferenceDepth: 5,
			},
		},
	}

	procInfo := &ditypes.ProcessInfo{
		PID:        0,
		BinaryPath: "blahblahblah",
		TypeMap: &ditypes.TypeMap{
			Functions: map[string][]*ditypes.Parameter{
				"compilerError": {
					{
						Name:     "a",
						Type:     "int",
						Kind:     uint(reflect.Int),
						Location: &ditypes.Location{},
						LocationExpressions: []ditypes.LocationExpression{
							ditypes.CompilerErrorLocationExpression(),
						},
					},
				},
				"verifierError": {
					{
						Name:     "a",
						Type:     "int",
						Kind:     uint(reflect.Int),
						Location: &ditypes.Location{},
						LocationExpressions: []ditypes.LocationExpression{
							ditypes.VerifierErrorLocationExpression(),
						},
					},
				},
			},
		},
	}

	go func() {
		// Need to consume diagnostics updates to avoid blocking
		for {
			<-diagnostics.Diagnostics.Updates
		}
	}()

	applyConfigUpdate(procInfo, compilerErrorProbe)
	if compilerErrorProbe.InstrumentationInfo.InstrumentationOptions.CaptureParameters != false {
		t.Errorf("expected capture parameters to be false, got true")
	}
	if compilerErrorProbe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth != 0 {
		t.Errorf("expected max reference depth to be 0, got %d", compilerErrorProbe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth)
	}

	applyConfigUpdate(procInfo, verifierErrorProbe)
	if verifierErrorProbe.InstrumentationInfo.InstrumentationOptions.CaptureParameters != false {
		t.Errorf("expected capture parameters to be false, got true")
	}
	if verifierErrorProbe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth != 0 {
		t.Errorf("expected max reference depth to be 0, got %d", verifierErrorProbe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth)
	}
}
