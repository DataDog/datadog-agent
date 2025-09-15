// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// RequestStatOSSpecific stores stats for HTTP requests to a particular path
type RequestStatOSSpecific struct{}

func (*RequestStatOSSpecific) merge(*RequestStatOSSpecific) {}

// GetDynamicTags is a no-op method for linux.
func (*RequestStat) GetDynamicTags() common.StringSet {
	return nil
}
