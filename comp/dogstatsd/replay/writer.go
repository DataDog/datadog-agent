// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"time"

	// Refactor relevant bits
	"github.com/DataDog/zstd"
	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const (
	fileTemplate = "datadog-capture-%d"
)

// CaptureBuffer holds pointers to captured packet's buffers (and oob buffer if required) and the protobuf
// message used for serialization.
type CaptureBuffer struct {
	Pb          pb.UnixDogstatsdMsg
	Oob         *[]byte
	Pid         int32
	ContainerID string
	Buff        *packets.Packet
}

// for testing purposes
//
//nolint:unused
type backendFs struct {
	fs afero.Fs

	sync.RWMutex
}

// captureFs, used exclusively for testing purposes
//
//nolint:unused
var captureFs = backendFs{
	fs: afero.NewOsFs(),
}

// CapPool is a pool of CaptureBuffer
var CapPool = sync.Pool{
	New: func() interface{} {
		return new(CaptureBuffer)
	},
}

// TrafficCaptureWriter allows writing dogstatsd traffic to a file.
type TrafficCaptureWriter struct {
	zWriter   *zstd.Writer
	writer    *bufio.Writer
	Traffic   chan *CaptureBuffer
	ongoing   bool
	accepting bool

	sharedPacketPoolManager *packets.PoolManager
	oobPacketPoolManager    *packets.PoolManager

	taggerState map[int32]string

	// Synchronizes access to ongoing, accepting and closing of Traffic
	sync.RWMutex
}

// NewTrafficCaptureWriter creates a TrafficCaptureWriter instance.
func NewTrafficCaptureWriter(depth int) *TrafficCaptureWriter {

	return &TrafficCaptureWriter{
		Traffic:     make(chan *CaptureBuffer, depth),
		taggerState: make(map[int32]string),
	}
}

// processMessage receives a capture buffer and writes it to disk while also tracking
// the PID map to be persisted to the taggerState. Should not normally be called directly.
func (tc *TrafficCaptureWriter) processMessage(msg *CaptureBuffer) error {
	panic("not called")
}

// validateLocation validates the location passed as an argument is writable.
// The location and/or and error if any are returned.
func validateLocation(fs afero.Fs, location string, defaultLocation string) (string, error) {
	panic("not called")
}

// OpenFile checks that location is acceptable for a capture and creates a new file using given fs implementation.
func OpenFile(fs afero.Fs, l string, defaultLocation string) (afero.File, string, error) {
	panic("not called")
}

// Capture start the traffic capture and writes the packets to file at the
// specified location and for the specified duration.
func (tc *TrafficCaptureWriter) Capture(target io.WriteCloser, d time.Duration, compressed bool) {
	panic("not called")
}

// StopCapture stops the ongoing capture if in process.
func (tc *TrafficCaptureWriter) StopCapture() {
	panic("not called")
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *TrafficCaptureWriter) Enqueue(msg *CaptureBuffer) bool {
	panic("not called")
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCaptureWriter.
func (tc *TrafficCaptureWriter) RegisterSharedPoolManager(p *packets.PoolManager) error {
	if tc.sharedPacketPoolManager != nil {
		return fmt.Errorf("OOB Pool Manager already registered with the writer")
	}

	tc.sharedPacketPoolManager = p

	return nil
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCaptureWriter.
func (tc *TrafficCaptureWriter) RegisterOOBPoolManager(p *packets.PoolManager) error {
	if tc.oobPacketPoolManager != nil {
		return fmt.Errorf("OOB Pool Manager already registered with the writer")
	}

	tc.oobPacketPoolManager = p

	return nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCaptureWriter instance.
func (tc *TrafficCaptureWriter) IsOngoing() bool {
	panic("not called")
}

// writeHeader writes the .dog file format header to the capture file.
func (tc *TrafficCaptureWriter) writeHeader() error {
	panic("not called")
}

// writeState writes the tagger state to the capture file.
func (tc *TrafficCaptureWriter) writeState() (int, error) {
	panic("not called")
}

// writeNext writes the next CaptureBuffer after serializing it to a protobuf format.
// Continuing writes after an error calling this function would result in a corrupted file
func (tc *TrafficCaptureWriter) writeNext(msg *CaptureBuffer) error {
	panic("not called")
}

// Write writes the byte slice argument to file.
func (tc *TrafficCaptureWriter) Write(p []byte) (int, error) {
	panic("not called")
}
