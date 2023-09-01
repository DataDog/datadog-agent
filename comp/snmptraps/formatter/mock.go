// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package formatter

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"

	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var MockModule = fxutil.Component(
	fx.Provide(func() Component {
		return &DummyFormatter{}
	}),
)

type DummyFormatter struct{}

// FormatPacket is a dummy formatter method that hashes an SnmpPacket object
func (f DummyFormatter) FormatPacket(packet *packet.SnmpPacket) ([]byte, error) {
	var b bytes.Buffer
	gob.NewEncoder(&b).Encode(packet.Addr)
	gob.NewEncoder(&b).Encode(packet.Content.Community)
	gob.NewEncoder(&b).Encode(packet.Content.SnmpTrap)
	gob.NewEncoder(&b).Encode(packet.Content.Variables)
	gob.NewEncoder(&b).Encode(packet.Content.Version)

	h := sha256.New()
	h.Write(b.Bytes())
	hexHash := hex.EncodeToString(h.Sum(nil))
	return []byte(hexHash), nil
}
