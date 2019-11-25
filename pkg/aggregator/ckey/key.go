// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package ckey

import (
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	"github.com/DataDog/mmh3"
)

const byteSize = 16

// ContextKey is a non-cryptographic hash that allows to
// aggregate metrics from a same context together.
//
// This implementation has been designed to remove all heap
// allocations from the intake to reduce GC pressure on high
// volumes.
//
// It uses the 128bit murmur3 hash, that is already successfully
// used on other products. 128bit is probably overkill for avoiding
// collisions, but it's better to err on the safe side, as we do not
// have a collision mitigation mechanism.
type ContextKey [byteSize]byte

// hashPool is a reusable pool of murmur hasher objects
// to avoid allocating
var hashPool = sync.Pool{
	New: func() interface{} {
		return &mmh3.HashWriter128{}
	},
}

// Generate returns the ContextKey hash for the given parameters.
// The tags array is sorted in place to avoid heap allocations.
func Generate(name, hostname string, tags []string) ContextKey {
	mmh := hashPool.Get().(*mmh3.HashWriter128)
	mmh.Reset()
	defer hashPool.Put(mmh)

	// Sort the tags in place. For typical tag slices, we use
	// the in-place section sort to avoid heap allocations.
	// We default to stdlib's sort package for longer slices.
	if len(tags) < 20 {
		selectionSort(tags)
	} else {
		sort.Strings(tags)
	}

	mmh.WriteString(name)
	mmh.WriteString(",")
	for _, t := range tags {
		mmh.WriteString(t)
		mmh.WriteString(",")
	}
	mmh.WriteString(hostname)

	var hash [byteSize]byte
	mmh.Sum(hash[0:0])
	return hash
}

// Compare returns an integer comparing two strings lexicographically.
// The result will be 0 if a==b, -1 if a < b, and +1 if a > b.
func Compare(a, b ContextKey) int {
	for i := 0; i < byteSize; i++ {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		default: // equality, compare next byte
			continue
		}
	}
	return 0
}

// IsZero returns true if the key is at zero value
func (k ContextKey) IsZero() bool {
	for _, b := range k {
		if b != 0 {
			return false
		}
	}
	return true
}

// String returns the hexadecimal representation of the key
func (k ContextKey) String() string {
	return hex.EncodeToString(k[:])
}

// Parse returns a ContextKey instanciated with the value decoded as hexa
func Parse(src string) (ContextKey, error) {
	if hex.DecodedLen(len(src)) != byteSize {
		return ContextKey{}, fmt.Errorf("invalid source length, expected %d bytes", byteSize)
	}
	var key ContextKey
	_, err := hex.Decode(key[:], []byte(src))
	return key, err
}
