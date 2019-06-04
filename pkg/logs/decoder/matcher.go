// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import "bytes"

// EndLineMatcher defines the criterion to whether to end a line or not.
type EndLineMatcher interface {
	Match(buffer *bytes.Buffer, bs []byte, start int, end int) bool
}

type newLineMatcher struct {
	EndLineMatcher
}

func (n *newLineMatcher) Match(buffer *bytes.Buffer, bs []byte, start int, end int) bool {
	return bs[end] == '\n'
}
