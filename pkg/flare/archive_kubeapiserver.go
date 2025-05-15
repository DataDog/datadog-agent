// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package flare

import (
	"encoding/json"
	"io"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	autoscalingWorkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
)

// GetAutoscalerList fetches the autoscaler list from the given URL.
func GetAutoscalerList(remoteURL string) ([]byte, error) {
	c := apiutil.GetClient(apiutil.WithInsecureTransport) // FIX IPC: get certificates right then remove this option

	r, err := apiutil.DoGet(c, remoteURL, apiutil.LeaveConnectionOpen)
	if err != nil {
		return nil, err
	}

	autoscalerDump := autoscalingWorkload.AutoscalersInfo{}
	err = json.Unmarshal(r, &autoscalerDump)

	if err != nil {
		return nil, err
	}

	fct := func(w io.Writer) error {
		autoscalerDump.Print(w)
		return nil
	}
	return functionOutputToBytes(fct), nil
}
