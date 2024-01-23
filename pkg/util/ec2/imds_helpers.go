// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"time"
)

var (
	imdsInstanceID = "/instance-id"
	imdsHostname   = "/hostname"
	// This is used in ec2_tags.go which is behind the 'ec2' build flag
	imdsTags        = "/tags/instance" //nolint:unused
	imdsIPv4        = "/public-ipv4"
	imdsNetworkMacs = "/network/interfaces/macs"
)

func getToken(ctx context.Context) (string, time.Time, error) {
	panic("not called")
}

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, forceIMDSv2 bool) (string, error) {
	panic("not called")
}

func getMetadataItem(ctx context.Context, endpoint string, forceIMDSv2 bool) (string, error) {
	panic("not called")
}

func doHTTPRequest(ctx context.Context, url string, forceIMDSv2 bool) (string, error) {
	panic("not called")
}
