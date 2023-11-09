// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package formatterimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/require"
)

func TestMockFormatter(t *testing.T) {
	formatter := fxutil.Test[formatter.Component](t, MockModule)
	packet := packet.CreateTestV1GenericPacket()
	// we don't check the value itself because it uses "encoding/gob", which
	// produces different values depending on the platform.
	_, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
}
