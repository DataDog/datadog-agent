// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && !trivy
// +build containerd,!trivy

package containerd

import (
	"context"
)

func (c *collector) startSBOMCollection(context.Context) error {
	return nil
}
