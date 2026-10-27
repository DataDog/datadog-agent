// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package daemon

import (
	"context"
	"net"
)

// rootOnlyChanges is unset outside macOS: on Linux the socket stays owned by root, which the
// daemon runs as, and on Windows the named pipe carries its own access control.
const rootOnlyChanges = false

// connContext is nil outside macOS: requests carry no caller uid, since nothing checks it.
var connContext func(context.Context, net.Conn) context.Context
