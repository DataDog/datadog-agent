// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcprotocoltestimpl

import (
	"testing"
)

// TestALPNFunctionsExist verifies that the ALPN websocket functions
// are implemented and accessible for testing. These functions set up
// websocket clients with ALPN protocol negotiation for dd-rc-v1.
//
// The actual ALPN protocol negotiation is tested end-to-end with real
// backend servers that support the dd-rc-v1 application protocol.
func TestALPNFunctionsExist(t *testing.T) {
	_ = newWebSocketClientWithALPN
	_ = runEchoLoopWithALPN
	_ = runWebSocketTestWithALPN
}
