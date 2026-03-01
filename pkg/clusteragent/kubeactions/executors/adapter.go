// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
)

// Executor is the interface that all executors in this package implement
type Executor interface {
	Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult
}
