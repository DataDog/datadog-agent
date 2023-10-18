// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package network

// GetStaticTags return the string list of static tags from network.ConnectionStats.Tags
func GetStaticTags(staticTags uint64) (tags []string) { //nolint:revive // TODO fix revive unused-parameter
	return tags
}

func IsTLSTag(staticTags uint64) bool { //nolint:revive // TODO fix revive unused-parameter
	return false
}
