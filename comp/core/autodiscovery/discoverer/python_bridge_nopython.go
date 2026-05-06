// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package discoverer

// NewPythonBridge returns nil in no-Python builds. discoverer.New(nil)
// returns a nil Discoverer; configmgr nil-checks before every call, so
// templates with Discovery set fail-closed (not scheduled) without
// retry traffic.
func NewPythonBridge() Bridge {
	return nil
}
