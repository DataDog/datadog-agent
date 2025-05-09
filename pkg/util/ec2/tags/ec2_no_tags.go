// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ec2

package tags

import "context"

// GetTags grabs the host tags from the EC2 api
func GetTags(_ context.Context) ([]string, error) {
	return []string{}, nil
}

func fetchTagsFromCache(_ context.Context) ([]string, error) {
	return []string{}, nil
}

// GetInstanceInfo collects information about the EC2 instance as host tags. This mimic the tags set by the AWS
// integration in Datadog backend allowing customer to collect those information without having to enable the crawler.
func GetInstanceInfo(_ context.Context) ([]string, error) {
	return []string{}, nil
}
