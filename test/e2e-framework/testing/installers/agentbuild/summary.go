// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Summary is an output receipt index, not an input fingerprint or cache key.
type Summary struct{ ID, Version, Image string }

func (r Result) Summary() Summary {
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(data)
	s := Summary{ID: hex.EncodeToString(sum[:])}
	if r.Image != nil {
		s.Image = r.Image.Delivered
	}
	if r.Package != nil {
		s.Version = r.Package.Version
	}
	return s
}
