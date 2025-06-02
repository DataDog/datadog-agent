// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var defaultDumper storeDumper

type storeDumper struct {
	store *store
}

// InitDumper initializes the default dumper with a given store
func InitDumper(store *store) {
	defaultDumper.store = store
}

// AutoscalersInfo is used to dump the autoscaling store content
type AutoscalersInfo struct {
	PodAutoscalers []model.PodAutoscalerInternal `json:"pod_autoscalers"`
}

// Print writes the autoscaling store content to a given writer in a human-readable format
func (info *AutoscalersInfo) Print(writer io.Writer) {
	if info == nil {
		return
	}

	if writer != color.Output {
		color.NoColor = true
	}

	for _, autoscaler := range info.PodAutoscalers {
		fmt.Fprintf(writer, "\n=== PodAutoscaler %s ===\n", color.GreenString(autoscaler.ID()))

		// Use the String() method of PodAutoscalerInternal
		fmt.Fprintln(writer, autoscaler.String(true))

		fmt.Fprintln(writer, "===")
	}
}

// Dump returns the autoscaling store content
func Dump() *AutoscalersInfo {
	if !pkgconfigsetup.Datadog().GetBool("autoscaling.workload.enabled") {
		log.Debug("Autoscaling is disabled")
		return nil
	}

	if defaultDumper.store == nil {
		log.Debug("Autoscaling store dumper is not initialized")
		return nil
	}

	datadogPodAutoscalers := defaultDumper.store.GetAll()

	log.Debugf("Found %d pod autoscalers", len(datadogPodAutoscalers))

	response := AutoscalersInfo{
		PodAutoscalers: datadogPodAutoscalers,
	}

	return &response
}
