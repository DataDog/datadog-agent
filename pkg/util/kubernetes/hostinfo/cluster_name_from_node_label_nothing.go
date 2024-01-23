// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet && !kubeapiserver

package hostinfo

import (
	"context"
)

// getANodeLabels returns nothing
//
//nolint:revive // TODO(CINT) Fix revive linter
func (_ *NodeInfo) getANodeLabels(_ context.Context) (map[string]string, error) {
	panic("not called")
}
