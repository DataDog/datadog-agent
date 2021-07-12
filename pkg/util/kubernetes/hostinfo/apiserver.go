// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package hostinfo

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const (
	apiserverTimeout = 10 * time.Second
)

func apiserverNodeLabels(nodeName string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), apiserverTimeout)
	defer cancel()

	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.NodeLabels(nodeName)
}
