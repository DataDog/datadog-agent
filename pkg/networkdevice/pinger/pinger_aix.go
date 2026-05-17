// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package pinger

import "errors"

// New creates a new pinger; not supported on AIX.
func New(_ Config) (Pinger, error) {
	return nil, errors.New("pinger not supported on AIX")
}
