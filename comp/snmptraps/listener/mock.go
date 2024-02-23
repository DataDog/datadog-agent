// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
)

// MockComponent just holds a channel to which tests can write.
type MockComponent interface {
	Component
	Send(*packet.SnmpPacket)
}
