// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compression

import (
	"bytes"

	"github.com/DataDog/zstd"
)

const ZstdEncoding = "zstd"

type ZstdStrategy struct {
}

func NewZstdStrategy() *ZstdStrategy {
	return &ZstdStrategy{}
}

func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	return zstd.Compress(nil, src)
}

func (s *ZstdStrategy) Decompress(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

func (s *ZstdStrategy) CompressBound(sourceLen int) int {
	return zstd.CompressBound(sourceLen)
}

func (s *ZstdStrategy) ContentEncoding() string {
	return ZstdEncoding
}

func NewZstdZipper(output *bytes.Buffer) *zstd.Writer {
	return zstd.NewWriter(output)
}
