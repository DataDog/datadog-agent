// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestIRGen(t *testing.T) {
	sampleServicePath := testutil.BuildSampleService(t)
	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	symbols, err := binary.Symbols()
	require.NoError(t, err)
	defer binary.Close()
	var probes []config.Probe
	for i, s := range symbols {
		if strings.HasPrefix(s.Name, "type:.") {
			continue
		}
		probes = append(probes, &config.LogProbe{
			ID: fmt.Sprintf("probe_%d", i),
			Where: &config.Where{
				MethodName: s.Name,
			},
		})
	}

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)
	v, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, v)
	// TODO(XXX): Validate some properties of the output program.
}
