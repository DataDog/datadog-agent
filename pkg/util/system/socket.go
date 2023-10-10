// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package system

import (
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

// CheckSocketAvailable returns if a socket at path is available
// first boolean returns if socket path exists
// second boolean returns if socket is reachable
var CheckSocketAvailable = socket.CheckSocketAvailable
