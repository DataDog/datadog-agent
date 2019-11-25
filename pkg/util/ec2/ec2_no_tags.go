// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !ec2

package ec2

// GetTags grabs the host tags from the EC2 api
func GetTags() ([]string, error) {
	return []string{}, nil
}
