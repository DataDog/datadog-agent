// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import "strings"

// goldenCases cover every branch of BuildIssue. Their rendered issues are pinned
// in testdata/golden so a refactor that changes what customers read fails.
var goldenCases = map[string]map[string]string{
	"several_sources": {
		contextKeyBytes: "4200512", contextKeyRotations: "4", contextKeySourceCount: "3",
		contextKeyPairsOmitted: "0", contextKeyLastLossAt: "2026-11-16T10:00:00Z",
		contextKeySources: `[{"source":"nginx","service":"web","bytes":4000000,"rotations":2},{"source":"redis","service":"cache","bytes":200000,"rotations":1},{"source":"kafka","service":"queue","bytes":512,"rotations":1}]`,
	},
	"single_named_source": {
		contextKeyBytes: "1024", contextKeyRotations: "1", contextKeySourceCount: "1",
		contextKeyPairsOmitted: "0", contextKeyLastLossAt: "2026-11-16T10:00:00Z",
		contextKeySources: `[{"source":"nginx","service":"web","bytes":1024,"rotations":1}]`,
	},
	"pairs_omitted": {
		contextKeyBytes: "99999", contextKeyRotations: "12", contextKeySourceCount: "14",
		contextKeyPairsOmitted: "4", contextKeyLastLossAt: "2026-11-16T10:00:00Z",
		contextKeySources: `[{"source":"a","service":"a","bytes":90000,"rotations":10}]`,
	},
	"unsafe_names": {
		contextKeyBytes: "10", contextKeyRotations: "1", contextKeySourceCount: "1",
		contextKeyPairsOmitted: "0", contextKeyLastLossAt: "2026-11-16T10:00:00Z",
		contextKeySources: `[{"source":"ngi\nnx` + strings.Repeat("é", 80) + `","service":"","bytes":10,"rotations":1}]`,
	},
	"malformed_sources": {
		contextKeyBytes: "x", contextKeyRotations: "2", contextKeySourceCount: "2", contextKeySources: "{not json",
	},
	"empty": {},
	"nil":   nil,
}
