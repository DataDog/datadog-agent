// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import "bytes"

// prefixer prepends a prefix to a message.
type prefixer struct {
	get    func() string
	buffer bytes.Buffer
}

// newPrefixer returns a prefixer that will fetch the prefix and prepends it a given message each time apply is called.
func newPrefixer(getter func() string) *prefixer {
	return &prefixer{
		get: getter,
	}
}

// apply prepends the prefix and a space to the message.
func (p *prefixer) apply(content []byte) []byte {
	p.buffer.Reset()
	p.buffer.WriteString(p.get())
	p.buffer.WriteByte(' ')
	p.buffer.Write(content)
	return p.buffer.Bytes()
}
