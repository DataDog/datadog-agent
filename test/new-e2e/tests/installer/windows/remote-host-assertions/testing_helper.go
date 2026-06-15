// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"testing"

	e2ecommon "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// contextT extracts *testing.T from a common.Context via type assertion.
// These assertion helpers are always created in test context (BaseSuite or *testing.T),
// so this is safe. It panics if the context does not expose *testing.T.
func contextT(ctx e2ecommon.Context) *testing.T {
	type hasT interface{ T() *testing.T }
	if h, ok := ctx.(hasT); ok {
		return h.T()
	}
	panic("assertions: context does not expose *testing.T; only use these helpers in test context")
}
