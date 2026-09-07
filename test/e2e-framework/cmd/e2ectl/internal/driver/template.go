// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

// StarterConfig generates the selected registered schema's complete example.
func StarterConfig(base string) ([]byte, error) {
	d, err := Get(base)
	if err != nil {
		return nil, err
	}
	return d.StarterConfig()
}
