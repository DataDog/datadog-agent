// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build serverless

package replay

func (tc *TrafficCaptureWriter) writeState() (int, error) {
	// nothing to do here
	return 0, nil
}
