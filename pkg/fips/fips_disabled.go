// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !goexperiment.systemcrypto

package fips

func Status() string {
	return ""
}

func Enabled() bool {
	return false
}
