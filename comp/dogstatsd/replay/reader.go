// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"io"
	"sync" // might be unnecessary

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// TrafficCaptureReader allows reading back a traffic capture and its contents
type TrafficCaptureReader struct {
	Contents    []byte
	rawContents []byte
	Version     int
	Traffic     chan *pb.UnixDogstatsdMsg
	Done        chan struct{}
	fuse        chan struct{}
	offset      uint32
	mmap        bool

	sync.Mutex
}

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int, mmap bool) (*TrafficCaptureReader, error) {
	panic("not called")
}

// Read reads the contents of the traffic capture and writes each packet to a channel
func (tc *TrafficCaptureReader) Read(ready chan struct{}) {
	panic("not called")
}

// Close cleans up any resources used by the TrafficCaptureReader, should not normally
// be called directly.
func (tc *TrafficCaptureReader) Close() error {
	panic("not called")
}

// Shutdown triggers the fuse if there's an ongoing read routine, and closes the reader.
func (tc *TrafficCaptureReader) Shutdown() error {
	panic("not called")
}

// ReadNext reads the next packet found in the file and returns the protobuf representation and an error if any.
func (tc *TrafficCaptureReader) ReadNext() (*pb.UnixDogstatsdMsg, error) {
	panic("not called")
}

// Seek sets the reader to the specified offset. Please note,
// the specified offset is relative to the first datagram, not the
// absolute position in the file, that would include the header. Thus,
// an offset of 0 would be the first datagram. Use with caution, a bad
// offset will completely mess up a replay.
func (tc *TrafficCaptureReader) Seek(offset uint32) {
	panic("not called")
}

// ReadState reads the tagger state from the end of the capture file.
// The internal offset of the reader is not modified by this operation.
func (tc *TrafficCaptureReader) ReadState() (map[int32]string, map[string]*pb.Entity, error) {
	panic("not called")
}

// Read reads the next protobuf packet available in the file and returns it in a byte slice, and an error if any.
func Read(r io.Reader) ([]byte, error) {
	panic("not called")
}
