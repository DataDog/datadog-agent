// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build docker

package docker

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.LogSource, containerID string) *decoder.Decoder {
	return decoder.NewDecoderWithEndLineMatcher(source, NewParser(containerID), &headerMatcher{})
}

const (
	headerLength       = 8
	headerPrefixLength = 4
)

var (
	headerStdoutPrefix = []byte{1, 0, 0, 0}
	headerStderrPrefix = []byte{2, 0, 0, 0}
)

type headerMatcher struct {
	decoder.EndLineMatcher
}

// Match does an extra checking on matching docker header. The header should be
// ignored for determine weather it's a end of line or not.
func (s *headerMatcher) Match(exists []byte, appender []byte, start int, end int) bool {
	return appender[end] == '\n' && !s.matchHeader(exists, appender[start:end])
}

// When a newline (in byte is 10) is matching, an additional check need to
// be done to make sure this is not part of docker header.
// [1|2 0 0 0 size1 size2 size3 size4], where size1 size2 size3 size4 can be
// 10 in byte.
// case [1|2 0 0 0 10 size2 size3 size4]
// case [1|2 0 0 0 size1 10 size3 size4]
// case [1|2 0 0 0 size1 size2 10 size4]
// case [1|2 0 0 0 size1 size2 size3 10]
func (s *headerMatcher) matchHeader(exists []byte, bs []byte) bool {
	l := len(exists) + len(bs)
	if l >= headerLength || l < headerPrefixLength {
		return false
	}
	h := append(exists, bs...)
	return bytes.HasPrefix(h, headerStdoutPrefix) ||
		bytes.HasPrefix(h, headerStderrPrefix)
}
