// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package events handles process events
package events

import (
	"go4.org/intern"
)

// Initialized returns true if Init() has been called successfully
func Initialized() bool {
	return false
}

func getAPMTags(_ map[string]struct{}, _ string) []*intern.Value {
	return nil
}