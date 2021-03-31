// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync" // might be unnecessary
	"time"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/replay/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
	"github.com/h2non/filetype"
)

// TrafficCaptureReader allows reading back a traffic capture and its contents
type TrafficCaptureReader struct {
	Contents []byte
	Traffic  chan *pb.UnixDogstatsdMsg
	Shutdown chan struct{}
	offset   uint32
	last     int64

	sync.Mutex
}

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int) (*TrafficCaptureReader, error) {

	// MMap file so that we can have reasonable performance with very large files
	c, err := getFileMap(path)
	if err != nil {
		return nil, err
	}

	// datadog capture file should be already registered with filetype via the init hooks
	kind, _ := filetype.Match(c)
	if kind == filetype.Unknown {
		return nil, fmt.Errorf("unknown capture file provided")
	}

	return &TrafficCaptureReader{
		Contents: c,
		Traffic:  make(chan *pb.UnixDogstatsdMsg, depth),
	}, nil
}

// Read reads the contents of the traffic capture and writes each packet to a channel
func (tc *TrafficCaptureReader) Read() {
	tc.Shutdown = make(chan struct{})
	defer close(tc.Shutdown)

	for {
		msg, err := tc.ReadNext()
		if err != nil && err == io.EOF {
			log.Debugf("Done reading capture file...", err)
			break
		} else if err != nil {
			log.Errorf("Error processing: %v", err)
			break
		}

		// TODO: ensure proper cadence
		if tc.last != 0 {
			if msg.Timestamp > tc.last {
				time.Sleep(time.Second * time.Duration(msg.Timestamp-tc.last))
			}
		}

		tc.last = msg.Timestamp
		tc.Traffic <- msg
	}
}

// Close cleans up any resources used by the TrafficCaptureReader
func (tc *TrafficCaptureReader) Close() error {
	return unmapFile(tc.Contents)
}

// ReadNext reads the next packet found in the file and returns the protobuf representation and an error if any.
func (tc *TrafficCaptureReader) ReadNext() (*pb.UnixDogstatsdMsg, error) {

	tc.Lock()

	if int(tc.offset+4) > len(tc.Contents) {
		return nil, io.EOF
	}
	sz := binary.LittleEndian.Uint32(tc.Contents[tc.offset : tc.offset+4])
	tc.offset += 4

	if int(tc.offset+sz) > len(tc.Contents) {
		return nil, io.EOF
	}

	// avoid a fresh allocation - at least this runs in a separate process
	msg := &pb.UnixDogstatsdMsg{}
	err := proto.Unmarshal(tc.Contents[tc.offset:tc.offset+sz], msg)
	if err != nil {
		return nil, err
	}
	tc.offset += sz

	tc.Unlock()

	return msg, nil
}
