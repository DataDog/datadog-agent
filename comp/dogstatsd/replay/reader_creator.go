// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build !serverless

package replay

import (
	"fmt"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/zstd"
	"github.com/h2non/filetype"
)

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int, mmap bool) (*TrafficCaptureReader, error) {

	c, err := getFileContent(path, mmap)
	if err != nil {
		fmt.Printf("Unable to map file: %v\n", err)
		return nil, err
	}

	// datadog capture file should be already registered with filetype via the init hooks
	kind, _ := filetype.Match(c)
	if kind == filetype.Unknown {
		return nil, fmt.Errorf("unknown capture file provided: %v", kind.MIME)
	}

	decompress := false
	if kind.MIME.Subtype == "zstd" {
		decompress = true
		log.Debug("capture file compressed with zstd")
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
