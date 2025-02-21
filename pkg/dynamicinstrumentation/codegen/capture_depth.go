// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen is used to generate bpf program source code based on probe definitions
package codegen

import (
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type captureDepthItem struct {
	depth     int
	parameter *ditypes.Parameter
}

func applyCaptureDepth(params []*ditypes.Parameter, targetDepth int) {
	queue := []*captureDepthItem{}
	for i := range params {
		queue = append(queue, &captureDepthItem{
			depth:     0,
			parameter: params[i],
		})
	}
	for len(queue) > 0 {
		top := queue[0]
		queue = queue[1:]

		if top.depth >= targetDepth {
			top.parameter.DoNotCapture = true
			top.parameter.NotCaptureReason = ditypes.CaptureDepthReached
		}

		for _, child := range top.parameter.ParameterPieces {
			queue = append(queue, &captureDepthItem{depth: top.depth + 1, parameter: child})
		}
	}
}
