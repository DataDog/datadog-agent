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

const testIdentityVersion = "testIdentity/v1"

func makeTestIdentity(sourceHostname string, pathtest common.Pathtest) string {
	if sourceHostname == "" {
		return ""
	}

	h := sha256.New()
	writeTestIdentityString(h, testIdentityVersion)
	writeTestIdentityString(h, string(pathtest.Protocol))
	writeTestIdentityString(h, sourceHostname)
	writeTestIdentityString(h, pathtest.Hostname)
	_ = binary.Write(h, binary.LittleEndian, pathtest.Port)

	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func writeTestIdentityString(h hash.Hash, value string) {
	_ = binary.Write(h, binary.LittleEndian, uint64(len(value)))
	_, _ = h.Write([]byte(value))
}
