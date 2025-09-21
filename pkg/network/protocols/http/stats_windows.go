// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build npm

package http

import (
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// RequestStatOSSpecific stores stats for HTTP requests to a particular path.
type RequestStatOSSpecific struct {
	// DynamicTags associated with the request.
	DynamicTags common.StringSet
}

func (r *RequestStatOSSpecific) merge(other *RequestStatOSSpecific) {
	if len(other.DynamicTags) != 0 {
		if r.DynamicTags == nil {
			r.DynamicTags = common.NewStringSet()
		}
		for tag := range other.DynamicTags {
			r.DynamicTags.Add(tag)
		}
	}
}

// GetDynamicTags returns the dynamic tags of the Windows version.
func (r *RequestStat) GetDynamicTags() common.StringSet {
	return r.RequestStatOSSpecific.DynamicTags
}
