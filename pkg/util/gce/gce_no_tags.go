// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !gce

package gce

// GetTags gets the tags from the GCE api
func GetTags() ([]string, error) {
	tags := []string{}

	return tags, nil
}
