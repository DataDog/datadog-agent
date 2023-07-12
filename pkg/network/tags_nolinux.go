// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package network

// GetStaticTags return the string list of static tags from network.ConnectionStats.Tags
func GetStaticTags(staticTags uint64) (tags []string) {
	return tags
}

func IsTLSTag(staticTags uint64) bool {
	return false
}
