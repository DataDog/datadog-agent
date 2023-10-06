// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !gce

package gce

import "context"

// GetTags gets the tags from the GCE api
func GetTags(ctx context.Context) ([]string, error) {
	tags := []string{}

	return tags, nil
}
