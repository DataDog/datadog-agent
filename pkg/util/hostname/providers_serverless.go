// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package hostname

import "context"

// IsConfigurationProvider returns false for serverless
func (h Data) FromConfiguration() bool {
	return false
}

// GetWithProvider returns an empty Data for serverless
func GetWithProvider(ctx context.Context) (Data, error) {
	return Data{}, nil
}

// Get returns an empty strig for serverless
func Get(ctx context.Context) (string, error) {
	return "", nil
}
