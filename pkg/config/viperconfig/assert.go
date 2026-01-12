// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !test

package viperconfig

import (
	"fmt"
)

func (c *safeConfig) assertIsTest(methodName string) {
	panic(fmt.Errorf("assertion failed: can only call %s from test code", methodName))
}
