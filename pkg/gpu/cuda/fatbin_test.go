// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFatbinFromPath(t *testing.T) {
	//path := "/home/gjulianm/cuda-samples/Samples/0_Introduction/matrixMul/matrixMul"
	path := "/home/gjulianm/.local/lib/python3.10/site-packages/torch/lib/libtorch_cuda.so"
	res, err := ParseFatbinFromELFFilePath(path)
	require.NoError(t, err)
	fmt.Printf("Fatbin: %v\n", res)

	for _, kern := range res.Kernels {
		fmt.Printf("Kernel: %+v\n", kern)
	}
}
