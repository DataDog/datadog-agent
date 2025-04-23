// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AutoscalingDumpResponse is used to dump the autoscaling store content
type AutoscalingDumpResponse struct {
	PodAutoscalers []*model.PodAutoscalerInternal `json:"pod_autoscalers"`
}

func Dump(ctx context.Context) *AutoscalingDumpResponse {
	datadogPodAutoscalers := GetAutoscalingStore(ctx).GetAll()

	datadogPodAutoscalerAddr := []*model.PodAutoscalerInternal{}

	log.Debugf("Found %d pod autoscalers", len(datadogPodAutoscalers))
	for _, podAutoscaler := range datadogPodAutoscalers {
		datadogPodAutoscalerAddr = append(datadogPodAutoscalerAddr, &podAutoscaler)
	}

	response := AutoscalingDumpResponse{
		PodAutoscalers: datadogPodAutoscalerAddr,
	}

	return &response
}

// Write writes the store content to a given writer
func (adr *AutoscalingDumpResponse) Write(writer io.Writer) {
	if adr == nil {
		return
	}

	if writer != color.Output {
		color.NoColor = true
	}

	for _, autoscaler := range adr.PodAutoscalers {
		fmt.Fprintf(writer, "\n=== PodAutoscaler %s ===\n", color.GreenString(autoscaler.ID()))

		// Use the String() method of PodAutoscalerInternal
		fmt.Fprintln(writer, autoscaler.String(true))

		fmt.Fprintln(writer, "===")
	}
}
