// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package secrets

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func checkRights(path string) error {
	log.Warn("The secrets feature is not yet safe to use on Windows. This beta build is for testing only. Use at your own risk.")
	return nil
}
