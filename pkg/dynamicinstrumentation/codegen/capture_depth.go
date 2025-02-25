// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package codegen is used to generate bpf program source code based on probe definitions
package codegen

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

type captureDepthItem struct {
	depth     int
	parameter *ditypes.Parameter
}

func applyCaptureDepth(params []*ditypes.Parameter, targetDepth int) {
	// fmt.Println("Entry:", pretty.Sprint(params))
	queue := []*captureDepthItem{}
	for i := range params {
		if params[i] == nil {
			continue
		}
		queue = append(queue, &captureDepthItem{
			depth:     0,
			parameter: params[i],
		})
	}
	for len(queue) > 0 {
		top := queue[0]
		queue = queue[1:]

		if top.depth >= targetDepth {
			fmt.Println("not capturing: ", top.parameter.Name)
			top.parameter.DoNotCapture = true
			top.parameter.NotCaptureReason = ditypes.CaptureDepthReached
		}

		if top.parameter.Kind == uint(reflect.Struct) {
			for _, child := range top.parameter.ParameterPieces {
				queue = append(queue, &captureDepthItem{depth: top.depth, parameter: child})
			}
			continue
		}

		if top.parameter.Kind == uint(reflect.Slice) {
			// If we do want to capture the slice, it means that we at least capture the first
			// layer of the slice elements. Only this layer counts towards capture depth, but
			// if it's set to DoNotCapture, then this slice layer header should be set to it as well
			if len(top.parameter.ParameterPieces) == 0 || top.parameter.ParameterPieces[0] == nil ||
				len(top.parameter.ParameterPieces[0].ParameterPieces) == 0 ||
				top.parameter.ParameterPieces[0].ParameterPieces[0] == nil {
				fmt.Println("CONTINUING!!!!!!!!!")
				continue
			}
			elementType := top.parameter.ParameterPieces[0].ParameterPieces[0]
			queue = append(queue, &captureDepthItem{depth: top.depth, parameter: elementType})
			continue
		}

		for _, child := range top.parameter.ParameterPieces {
			queue = append(queue, &captureDepthItem{depth: top.depth + 1, parameter: child})
		}
	}

	// fmt.Println("Exit:", pretty.Sprint(params))
}
