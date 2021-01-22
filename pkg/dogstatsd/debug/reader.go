// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package debug

import (
	"bufio"
	"encoding/binary"
	"os"
	"sync" // might be unnecessary

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/debug/pb"

	proto "github.com/golang/protobuf/proto"
)

type TrafficCaptureReader struct {
	File     *os.File
	reader   *bufio.Reader
	Traffic  chan *pb.UnixDogstatsdMsg
	Shutdown chan struct{}
	sync.Mutex
}

func NewTrafficCaptureReader(path string, depth int) (*TrafficCaptureReader, error) {

	fp, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return &TrafficCaptureReader{
		File:    fp,
		reader:  bufio.NewReader(fp),
		Traffic: make(chan *pb.UnixDogstatsdMsg, depth),
	}, nil
}

func (tc *TrafficCaptureReader) Read() {
	tc.Shutdown = make(chan struct{})
	defer close(tc.Shutdown)

	for {
		msg, err := tc.ReadNext()
		if err != nil {
			break
		}

		// TODO: ensure proper cadence
		tc.Traffic <- msg
	}
}

func (tc *TrafficCaptureReader) ReadNext() (*pb.UnixDogstatsdMsg, error) {

	d, err := tc.reader.Peek(4)
	if err != nil {
		return nil, err
	}

	sz := binary.LittleEndian.Uint32(d)
	tc.reader.Discard(4)

	d, err = tc.reader.Peek(int(sz))
	defer tc.reader.Discard(int(sz))

	if err != nil {
		return nil, err
	}

	// avoid a fresh allocation
	msg := &pb.UnixDogstatsdMsg{}
	err = proto.Unmarshal(d, msg)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
