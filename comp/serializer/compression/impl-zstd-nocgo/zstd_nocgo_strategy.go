// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides a set of functions for compressing with zstd
package zstdimpl

import (
	"bytes"
	"os"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/klauspost/compress/zstd"

	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level int
}

// Provides contains the compression component
type Provides struct {
	Comp compression.Component
}

var globalencoder *zstd.Encoder
var mutex sync.Mutex

// ZstdNoCgoStrategy can be manually selected via component - it's not used by any selector / config option
type ZstdNoCgoStrategy struct {
	level   int
	encoder *zstd.Encoder
}

// NewComponent returns a new ZstdNoCgoStrategy
func NewComponent(reqs Requires) Provides {
	log.Debugf("Compressing native zstd at level %d", reqs.Level)

	mutex.Lock()
	if globalencoder == nil {
		conc, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_CONCURRENCY"))
		if err != nil {
			conc = 1
		}

		window, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_WINDOW"))
		if err != nil {
			window = 1 << 15
		}

		log.Debugf("native zstd concurrency %d", conc)
		log.Debugf("native zstd window size %d", window)
		globalencoder, _ = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(reqs.Level)),
			zstd.WithEncoderConcurrency(conc),
			zstd.WithLowerEncoderMem(true),
			zstd.WithWindowSize(window))
	}
	mutex.Unlock()

	return Provides{
		Comp: &ZstdNoCgoStrategy{
			level:   reqs.Level,
			encoder: globalencoder,
		},
	}
}

// Compress will compress the data with zstd
func (s *ZstdNoCgoStrategy) Compress(src []byte) ([]byte, error) {
	return s.encoder.EncodeAll(src, nil), nil
}

// Decompress will decompress the data with zstd
func (s *ZstdNoCgoStrategy) Decompress(src []byte) ([]byte, error) {
	decoder, _ := zstd.NewReader(nil)
	return decoder.DecodeAll(src, nil)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdNoCgoStrategy) CompressBound(sourceLen int) int {
	return s.encoder.MaxEncodedSize(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdNoCgoStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdNoCgoStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	writer, _ := zstd.NewWriter(output, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(s.level)))
	return writer
}
