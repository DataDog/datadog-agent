// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package nodetreemodel

func (c *ntmConfig) assertIsTest(_ string) {
	// nothing to do, it's okay to call this method from a test
}
