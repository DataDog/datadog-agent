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

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
	"github.com/h2non/filetype"
)

// TrafficCaptureReader allows reading back a traffic capture and its contents
type TrafficCaptureReader struct {
	Contents []byte
	Version  int
	Traffic  chan *pb.UnixDogstatsdMsg
	Done     chan struct{}
	fuse     chan struct{}
	offset   uint32
	last     int64

	sync.Mutex
}

// NewTrafficCaptureReader creates a TrafficCaptureReader instance
func NewTrafficCaptureReader(path string, depth int) (*TrafficCaptureReader, error) {

	// MMap file so that we can have reasonable performance with very large files
	c, err := getFileMap(path)
	if err != nil {
		fmt.Printf("Unable to map file: %v\n", err)
		return nil, err
	}

	// datadog capture file should be already registered with filetype via the init hooks
	kind, _ := filetype.Match(c)
	if kind == filetype.Unknown {
		return nil, fmt.Errorf("unknown capture file provided")
	}

	ver, err := fileVersion(c)
	if err != nil {
		return nil, err
	}

	return &TrafficCaptureReader{
		Contents: c,
		Version:  ver,
		Traffic:  make(chan *pb.UnixDogstatsdMsg, depth),
	}, nil
}

// Read reads the contents of the traffic capture and writes each packet to a channel
func (tc *TrafficCaptureReader) Read() {
	tc.Done = make(chan struct{})
	tc.fuse = make(chan struct{})
	defer close(tc.Done)

	log.Debugf("Processing capture file of size: %d", len(tc.Contents))

	// skip header
	tc.Lock()
	tc.offset = uint32(len(datadogHeader))
	tc.Unlock()

	var tsResolution time.Duration
	if tc.Version < minNanoVersion {
		tsResolution = time.Second
	} else {
		tsResolution = time.Nanosecond
	}

	// The state must be read out of band, it makes zero sense in the context
	// of the replaying process, it must be pushed to the agent. We just read
	// and submit the packets here.
	for {
		msg, err := tc.ReadNext()
		if err != nil && err == io.EOF {
			log.Debugf("Done reading capture file...")
			break
		} else if err != nil {
			log.Errorf("Error processing: %v", err)
			break
		}

		if tc.last != 0 {
			if msg.Timestamp > tc.last {
				util.Wait(tsResolution * time.Duration(msg.Timestamp-tc.last))
			}
		}

		tc.last = msg.Timestamp
		tc.Traffic <- msg

		select {
		case <-tc.fuse:
			return
		default:
			continue
		}
	}
}

// Close cleans up any resources used by the TrafficCaptureReader
func (tc *TrafficCaptureReader) Close() error {
	return unmapFile(tc.Contents)
}

// Shutdown triggers the fuse if there's an ongoing read routine, and closes the reader.
func (tc *TrafficCaptureReader) Shutdown() error {
	if tc.fuse != nil {
		close(tc.fuse)
	}
	return tc.Close()
}

// ReadNext reads the next packet found in the file and returns the protobuf representation and an error if any.
func (tc *TrafficCaptureReader) ReadNext() (*pb.UnixDogstatsdMsg, error) {

	tc.Lock()

	if int(tc.offset+4) > len(tc.Contents) {
		tc.Unlock()
		return nil, io.EOF
	}
	sz := binary.LittleEndian.Uint32(tc.Contents[tc.offset : tc.offset+4])
	tc.offset += 4

	// we have reached the state separator or overflow
	if sz == 0 || int(tc.offset+sz) > len(tc.Contents) {
		tc.Unlock()
		return nil, io.EOF
	}

	// avoid a fresh allocation - at least this runs in a separate process
	msg := &pb.UnixDogstatsdMsg{}
	err := proto.Unmarshal(tc.Contents[tc.offset:tc.offset+sz], msg)
	if err != nil {
		tc.Unlock()
		return nil, err
	}
	tc.offset += sz

	tc.Unlock()

	return msg, nil
}

// ReadState reads the tagger state from the end of the capture file.
// The internal offset of the reader is not modified by this operation.
func (tc *TrafficCaptureReader) ReadState() (map[int32]string, map[string]*pb.Entity, error) {

	tc.Lock()
	defer tc.Unlock()

	if tc.Version < minStateVersion {
		return nil, nil, fmt.Errorf("The replay file is version: %v and does not contain a tagger state", tc.Version)
	}

	length := len(tc.Contents)
	sz := binary.LittleEndian.Uint32(tc.Contents[length-4 : length])

	log.Debugf("State bytes to be read: %v", sz)
	if sz == 0 {
		return nil, nil, nil
	}

	// pb state
	pbState := &pb.TaggerState{}
	err := proto.Unmarshal(tc.Contents[length-int(sz)-4:length-4], pbState)
	if err != nil {
		tc.Unlock()
		return nil, nil, err
	}

	return pbState.PidMap, pbState.State, err
}

// Read reads the next protobuf packet available in the file and returns it in a byte slice, and an error if any.
func Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	size := binary.LittleEndian.Uint32(buf)

	msg := make([]byte, size)

	_, err := io.ReadFull(r, msg)
	if err != nil {
		return nil, err
	}

	return msg, err
}
