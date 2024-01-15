// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

// noFramingMatcher considers the given bytes as already framed.
type noFramingMatcher struct{}

// FindFrame considers the given bytes buffer as one full frame.
//
//nolint:revive // TODO(AML) Fix revive linter
func (m *noFramingMatcher) FindFrame(buf []byte, seen int) ([]byte, int) {
	return buf, len(buf)
}
