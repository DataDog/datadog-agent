// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains miscellaneous utility functions
package utils

import (
	"context"
	"encoding/xml"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/types"
)

// GetAndUnmarshalXML downloads the data at `url`, verifies a checksum if provided, and unmarshals the XML to the provided type.
func GetAndUnmarshalXML[T any](ctx context.Context, httpClient *HTTPClient, url string, checksum *types.Checksum) (*T, error) {
	content, err := httpClient.GetWithChecksum(ctx, url, checksum)
	if err != nil {
		return nil, err
	}

	var res T
	if err := xml.Unmarshal(content, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
