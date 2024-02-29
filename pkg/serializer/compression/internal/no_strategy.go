// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression

import (
	"bytes"
)

type NoopStrategy struct {
}

func NewNoopStrategy() *NoopStrategy {
	return &NoopStrategy{}
}

func (s *NoopStrategy) Compress(src []byte) ([]byte, error) {
	return src, nil
}

func (s *NoopStrategy) Decompress(src []byte) ([]byte, error) {
	return src, nil
}

func (s *NoopStrategy) CompressBound(sourceLen int) int {
	return sourceLen
}

func (s *NoopStrategy) ContentEncoding() string {
	return ""
}

type NoopZipper struct{}

func (s NoopZipper) Write([]byte) (int, error) {
	return 0, nil
}

func (s NoopZipper) Flush() error {
	return nil
}

func (s NoopZipper) Close() error {
	return nil
}

func NewNoopZipper(output *bytes.Buffer) NoopZipper {
	return NoopZipper{}
}
