// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJournalJenkinsHash64MatchesLookup3Vectors(t *testing.T) {
	assert.Equal(t, uint64(0xdeadbeefdeadbeef), journalJenkinsHash64(nil))
	assert.Equal(t, uint64(0x17770551ce7226e6), journalJenkinsHash64([]byte("Four score and seven years ago")))
}

func TestJournalSipHash24MatchesReferenceVectors(t *testing.T) {
	var key journalID
	var message [64]byte
	for index := range key {
		key[index] = byte(index)
	}
	for index := range message {
		message[index] = byte(index)
	}

	vectors := []struct {
		length int
		hash   uint64
	}{
		{length: 0, hash: 0x726fdb47dd0e0e31},
		{length: 1, hash: 0x74f839c593dc67fd},
		{length: 7, hash: 0xab0200f58b01d137},
		{length: 8, hash: 0x93f5f5799a932462},
		{length: 15, hash: 0xa129ca6149be45e5},
		{length: 63, hash: 0x958a324ceb064572},
	}
	for _, vector := range vectors {
		assert.Equal(t, vector.hash, journalSipHash24(message[:vector.length], key), "length %d", vector.length)
	}
}
