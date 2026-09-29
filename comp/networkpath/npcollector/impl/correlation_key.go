// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
)

// correlationKeyVersion is a domain separator. Bump it if the set of inputs or
// their encoding changes, so keys from two Agent versions can never collide.
const correlationKeyVersion = "correlationKey/v1"

// makeCorrelationKey derives the identity shared by a CNM connection and the
// Network Path event for the test it schedules.
//
// The inputs are the four values that define the test the Agent will actually
// run, taken after normalisation: the protocol (which ICMP mode may have
// rewritten), the Agent's own hostname, the destination hostname, and the port.
// Both sides of the pivot compute it from the same pathtest, which is what makes
// the two pipelines agree without talking to each other.
//
// Returns empty when the Agent cannot resolve its own hostname. That is a real
// state, not an error: the test still runs and still emits an event, it just
// cannot be named, and the UI falls back to a filtered view.
func makeCorrelationKey(sourceHostname string, pathtest common.Pathtest) string {
	if sourceHostname == "" {
		return ""
	}

	h := sha256.New()
	writeCorrelationKeyString(h, correlationKeyVersion)
	writeCorrelationKeyString(h, string(pathtest.Protocol))
	writeCorrelationKeyString(h, sourceHostname)
	writeCorrelationKeyString(h, pathtest.Hostname)
	_ = binary.Write(h, binary.LittleEndian, pathtest.Port)

	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// writeCorrelationKeyString length-prefixes each string before hashing it, so
// that ("ab", "c") and ("a", "bc") cannot produce the same digest.
func writeCorrelationKeyString(h hash.Hash, value string) {
	_ = binary.Write(h, binary.LittleEndian, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}
