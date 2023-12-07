// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package listener

import (
	"strings"
)

// isConnClosedError returns true if the error is related to a closed connection,
// for more details, see: https://golang.org/src/internal/poll/fd.go#L18.
func isClosedConnError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
