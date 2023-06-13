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

	"github.com/DataDog/zstd"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
	"github.com/h2non/filetype"
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

// Read reads the contents of the traffic capture and writes each packet to a channel
func (tc *TrafficCaptureReader) Read(ready chan struct{}) {
	tc.Lock()
	tc.Done = make(chan struct{})
	tc.fuse = make(chan struct{})
	defer close(tc.Done)

	log.Debugf("Processing capture file of size: %d", len(tc.Contents))

	// skip header
	tc.offset = uint32(len(datadogHeader))

	var tsResolution time.Duration
	if tc.Version < minNanoVersion {
		tsResolution = time.Second
	} else {
		tsResolution = time.Nanosecond
	}
	tc.Unlock()

	last := int64(0)

	// we are all ready to go - let the caller know
	ready <- struct{}{}

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

		if last != 0 {
			if msg.Timestamp > last {
				util.Wait(tsResolution * time.Duration(msg.Timestamp-last))
			}
		}

		last = msg.Timestamp
		tc.Traffic <- msg

		select {
		case <-tc.fuse:
			return
		default:
			continue
		}
	}
}

// Close cleans up any resources used by the TrafficCaptureReader, should not normally
// be called directly.
func (tc *TrafficCaptureReader) Close() error {
	tc.Lock()
	defer tc.Unlock()

	// drop reference for GC
	tc.Contents = nil

	if tc.mmap {
		return unmapFile(tc.rawContents)
	}

	return nil
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

// Seek sets the reader to the specified offset. Please note,
// the specified offset is relative to the first datagram, not the
// absolute position in the file, that would include the header. Thus,
// an offset of 0 would be the first datagram. Use with caution, a bad
// offset will completely mess up a replay.
func (tc *TrafficCaptureReader) Seek(offset uint32) {

	tc.Lock()
	defer tc.Unlock()

	tc.offset = uint32(len(datadogHeader)) + offset

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
