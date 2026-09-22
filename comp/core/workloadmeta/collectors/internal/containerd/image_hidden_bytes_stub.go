// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && !linux

package containerd

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type hiddenBytesCollector struct{}

func (*collector) initHiddenBytesCollection()                                         {}
func (*collector) startHiddenBytesCollection(context.Context)                         {}
func (*collector) stopHiddenBytesCollection()                                         {}
func (*collector) enqueueHiddenBytesImageLocked(*workloadmeta.ContainerImageMetadata) {}
func (*collector) forgetHiddenBytesImageLocked(string)                                {}
func (*collector) retryHiddenBytesImageLocked(string)                                 {}
