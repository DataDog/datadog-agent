// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package debug

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"sync" // might be unnecessary
	"time"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/debug/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
)

type TrafficCaptureReader struct {
	Contents []byte
	Traffic  chan *pb.UnixDogstatsdMsg
	Shutdown chan struct{}
	offset   uint32
	last     int64

	sync.Mutex
}

func NewTrafficCaptureReader(path string, depth int) (*TrafficCaptureReader, error) {

	// TODO: think about the following approach
	// read entire thing into memory for performance reasons
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &TrafficCaptureReader{
		Contents: c,
		Traffic:  make(chan *pb.UnixDogstatsdMsg, depth),
	}, nil
}

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
