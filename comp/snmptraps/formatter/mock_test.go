// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package formatter

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockFormatter(t *testing.T) {
	formatter := fxutil.Test[Component](t, MockModule)
	packet := packet.CreateTestV1GenericPacket()
	result, err := formatter.FormatPacket(packet)
	require.NoError(t, err)
	assert.Equal(t, "b694272c60c9b553f16d66cfd353dfba1e559d86c7716e1c327cc1574482c7f7", string(result))
}
