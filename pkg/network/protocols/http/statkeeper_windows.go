// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func getPathBufferSize(c *config.Config) int {
	return int(c.HTTPMaxRequestFragment)
}
