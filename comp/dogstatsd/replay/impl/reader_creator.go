// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replayimpl

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/zstd"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int, mmap bool) (*TrafficCaptureReader, error) {

	c, err := getFileContent(path, mmap)
	if err != nil {
		fmt.Printf("Unable to map file: %v\n", err)
		return nil, err
	}

	decompress := bytes.HasPrefix(c, zstdMagic)
	if decompress {
		log.Debug("capture file compressed with zstd")
	} else if !datadogMatcher(c) {
		return nil, fmt.Errorf("unknown capture file provided, starting with: % x", c[:min(len(c), len(datadogHeader))])
	}

	var contents []byte
	if decompress {
		if contents, err = zstd.Decompress(nil, c); err != nil {
			return nil, err
		}
	} else {
		contents = c
	}

	ver, err := fileVersion(contents)
	if err != nil {
		return nil, err
	}

	return &TrafficCaptureReader{
		rawContents: c,
		Contents:    contents,
		Version:     ver,
		Traffic:     make(chan *pb.UnixDogstatsdMsg, depth),
		mmap:        mmap,
	}, nil
}
