// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tcp

// prefixer prepends a prefix to a message.
type prefixer struct {
	prefix []byte
}

// newPrefixer returns a prefixer that prepends the given prefix to a message.
func newPrefixer(prefix string) *prefixer {
	return &prefixer{
		prefix: append([]byte(prefix)),
	}
}

// apply prepends the prefix to the message.
func (p *prefixer) apply(content []byte) []byte {
	return append(p.prefix, content...)
}
