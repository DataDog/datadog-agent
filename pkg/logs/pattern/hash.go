// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pattern

import "hash/fnv"

// Hash returns the FNV-1a hash used to identify an exact token sequence.
func Hash(tokens []Token) uint64 {
	h := fnv.New64a()
	var b [1]byte
	for _, token := range tokens {
		b[0] = byte(token)
		_, _ = h.Write(b[:])
	}
	return h.Sum64()
}
