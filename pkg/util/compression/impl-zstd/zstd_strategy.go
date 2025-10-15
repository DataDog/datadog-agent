// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package zstdimpl provides a set of functions for compressing with zstd
package zstdimpl

import (
	"bytes"

	"github.com/DataDog/zstd"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Requires contains the compression level for zstd compression
type Requires struct {
	Level compression.ZstdCompressionLevel

	Strategy   int
	Chain      int
	Window     int
	Hash       int
	Searchlog  int
	Minmatch   int
	NumWorkers int
}

// ZstdStrategy is the strategy for when serializer_compressor_kind is zstd
type ZstdStrategy struct {
	level int

	strategy   int
	chain      int
	window     int
	hash       int
	searchlog  int
	minmatch   int
	numworkers int
}

func clamp(param zstd.CParameter, value int) int {
	low, high := zstd.GetBounds(param)

	if value < low {
		return low
	}

	if value > high {
		return high
	}

	return value
}

// New returns a new ZstdStrategy
func New(reqs Requires) compression.Compressor {
	if reqs.Strategy > 0 {
		return &ZstdStrategy{
			strategy:   clamp(zstd.CParamStrategy, reqs.Strategy),
			chain:      clamp(zstd.CParamChainLog, reqs.Chain),
			window:     clamp(zstd.CParamWindowLog, reqs.Window),
			hash:       clamp(zstd.CParamHashLog, reqs.Hash),
			searchlog:  clamp(zstd.CParamSearchLog, reqs.Searchlog),
			minmatch:   clamp(zstd.CParamMinMatch, reqs.Minmatch),
			numworkers: clamp(zstd.CParamNbWorkers, reqs.NumWorkers),
		}
	}

	return &ZstdStrategy{
		level: int(reqs.Level),
	}
}

// Compress will compress the data with zstd
func (s *ZstdStrategy) Compress(src []byte) ([]byte, error) {
	if s.level > 0 {
		return zstd.CompressLevel(nil, src, s.level)
	}

	var err error
	ctx := zstd.NewCtx()
	err = ctx.SetParameter(zstd.CParamStrategy, s.strategy)
	if err != nil {
		return nil, err
	}
	err = ctx.SetParameter(zstd.CParamChainLog, s.chain)
	if err != nil {
		return nil, err
	}
	err = ctx.SetParameter(zstd.CParamWindowLog, s.window)
	if err != nil {
		return nil, err
	}
	err = ctx.SetParameter(zstd.CParamHashLog, s.hash)
	if err != nil {
		return nil, err
	}
	err = ctx.SetParameter(zstd.CParamSearchLog, s.searchlog)
	if err != nil {
		return nil, err
	}
	err = ctx.SetParameter(zstd.CParamMinMatch, s.minmatch)
	if err != nil {
		return nil, err
	}

	return ctx.Compress2(nil, src)

}

// Decompress will decompress the data with zstd
func (s *ZstdStrategy) Decompress(src []byte) ([]byte, error) {
	return zstd.Decompress(nil, src)
}

// CompressBound returns the worst case size needed for a destination buffer when using zstd
func (s *ZstdStrategy) CompressBound(sourceLen int) int {
	return zstd.CompressBound(sourceLen)
}

// ContentEncoding returns the content encoding value for zstd
func (s *ZstdStrategy) ContentEncoding() string {
	return compression.ZstdEncoding
}

// NewStreamCompressor returns a new zstd Writer
func (s *ZstdStrategy) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	if s.level > 0 {
		return zstd.NewWriterLevel(output, s.level)
	}

	return zstd.NewWriterParamsDict(output, nil, func(set func(zstd.CParameter, int)) {
		set(zstd.CParamStrategy, s.strategy)
		set(zstd.CParamChainLog, s.chain)
		set(zstd.CParamWindowLog, s.window)
		set(zstd.CParamHashLog, s.hash)
		set(zstd.CParamSearchLog, s.searchlog)
		set(zstd.CParamMinMatch, s.minmatch)
		set(zstd.CParamNbWorkers, s.numworkers)
	})
}
