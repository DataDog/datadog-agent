// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package strategy provides a set of functions for compressing with zlib / zstd /gzip
package strategy

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/klauspost/compress/zstd"
)

var globalencoder *zstd.Encoder
var mutex sync.Mutex

// ZstdNativeStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdNativeStrategy struct {
	level   int
	encoder *zstd.Encoder
}

// NewZstdNativeStrategy returns a new ZstdStrategy
func NewZstdNativeStrategy(level int) *ZstdNativeStrategy {
	log.Debugf("Compressing native zstd at level %d", level)

	mutex.Lock()
	if globalencoder == nil {
		conc, err := strconv.Atoi(os.Getenv("WAKKAS_CONCURRENCY"))
		if err != nil {
			conc = 1
		}

		window, err := strconv.Atoi(os.Getenv("WAKKAS_WINDOW"))
		if err != nil {
			window = 1 << 15
		}

		log.Debugf("native zstd concurrency %d", conc)
		log.Debugf("native zstd window size %d", window)
		globalencoder, _ = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
			zstd.WithEncoderConcurrency(conc),
			zstd.WithLowerEncoderMem(true),
			zstd.WithWindowSize(window))
	}
	mutex.Unlock()

	return &ZstdNativeStrategy{
		level:   level,
		encoder: globalencoder,
	}
}

// Compress will compress the data with zstd
func (s *ZstdNativeStrategy) Compress(src []byte) ([]byte, error) {
	return s.encoder.EncodeAll(src, nil), nil
}

// Decompress will decompress the data with zstd
func (*ZstdNativeStrategy) Decompress(src []byte) ([]byte, error) {
	decoder, _ := zstd.NewReader(nil)
	return decoder.DecodeAll(src, nil)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdNativeStrategy) CompressBound(sourceLen int) int {
	return s.encoder.MaxEncodedSize(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (*ZstdNativeStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor creates a new zstd stream compressor
func (s *ZstdNativeStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	writer, _ := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)))
	if writer == nil {
		fmt.Println("wut wut wut")
	}
	return writer
}
