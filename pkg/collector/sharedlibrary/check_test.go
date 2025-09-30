// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharedlibrary

import "testing"

func TestRunCheck(t *testing.T) {
	testRunCheck(t)
}

func TestRunCheckWithNullSymbol(t *testing.T) {
	testRunCheckWithNullSymbol(t)
}
