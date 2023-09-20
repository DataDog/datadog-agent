// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import "github.com/DataDog/datadog-agent/pkg/util/system/socket"

// CheckSocketAvailable returns named pipe availability
// as on Windows, sockets do not exist
var CheckSocketAvailable = socket.CheckSocketAvailable
