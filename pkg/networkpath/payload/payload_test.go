// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package payload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestICMPMode(t *testing.T) {
	require.True(t, ICMPModeNone.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeTCP.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeUDP.ShouldUseICMP(ProtocolICMP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolICMP))

	require.False(t, ICMPModeNone.ShouldUseICMP(ProtocolTCP))
	require.True(t, ICMPModeTCP.ShouldUseICMP(ProtocolTCP))
	require.False(t, ICMPModeUDP.ShouldUseICMP(ProtocolTCP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolTCP))

	require.False(t, ICMPModeNone.ShouldUseICMP(ProtocolUDP))
	require.False(t, ICMPModeTCP.ShouldUseICMP(ProtocolUDP))
	require.True(t, ICMPModeUDP.ShouldUseICMP(ProtocolUDP))
	require.True(t, ICMPModeAll.ShouldUseICMP(ProtocolUDP))
}
