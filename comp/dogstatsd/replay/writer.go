// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	// Refactor relevant bits
	"github.com/DataDog/zstd"
	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/golang/protobuf/proto"
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
type backendFs struct {
	fs afero.Fs

	sync.RWMutex
}

// captureFs, used exclusively for testing purposes
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
	err := tc.writeNext(msg)

	if err != nil {
		return err
	}

	if msg.ContainerID != "" {
		tc.taggerState[msg.Pid] = msg.ContainerID
	}

	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.Put(msg.Buff)
	}

	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.Put(msg.Oob)
	}

	return nil
}

// validateLocation validates the location passed as an argument is writable.
// The location and/or and error if any are returned.
func validateLocation(fs afero.Fs, location string, defaultLocation string) (string, error) {
	useDefaultLocation := location == ""
	if useDefaultLocation {
		location = defaultLocation
	}

	s, err := fs.Stat(location)
	if os.IsNotExist(err) {
		if useDefaultLocation {
			err := fs.MkdirAll(location, 0755)
			if err != nil {
				return "", err
			}
		} else {
			return "", log.Errorf("specified location does not exist: %v ", err)
		}
	} else if !s.IsDir() {
		return "", log.Errorf("specified location is not a directory: %v ", location)
	}

	if !useDefaultLocation && s.Mode()&os.FileMode(2) == 0 {
		return "", log.Errorf("specified location (%v) is not world writable: %v", location, s.Mode())
	}

	return location, nil

}

// OpenFile checks that location is acceptable for a capture and creates a new file using given fs implementation.
func OpenFile(fs afero.Fs, l string, defaultLocation string) (afero.File, string, error) {
	location, err := validateLocation(fs, l, defaultLocation)
	if err != nil {
		return nil, "", err
	}

	p, err := filepath.Abs(path.Join(location, fmt.Sprintf(fileTemplate, time.Now().Unix())))
	if err != nil {
		return nil, "", err
	}

	f, err := fs.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_EXCL, 0660)
	if err != nil {
		return nil, "", err
	}

	return f, p, err
}

// Capture start the traffic capture and writes the packets to file at the
// specified location and for the specified duration.
func (tc *TrafficCaptureWriter) Capture(target io.WriteCloser, d time.Duration, compressed bool) {
	defer target.Close()
	log.Debug("Starting capture...")

	if compressed {
		tc.zWriter = zstd.NewWriter(target)
		tc.writer = bufio.NewWriter(tc.zWriter)
	} else {
		tc.zWriter = nil
		tc.writer = bufio.NewWriter(target)
	}

	tc.Lock()
	if tc.ongoing {
		log.Errorf("capture is already running")
	}
	tc.ongoing = true
	tc.accepting = true
	tc.Unlock()

	err := tc.writeHeader()
	if err != nil {
		log.Errorf("There was an issue writing the capture file header: %v ", err)

		return
	}

	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.SetPassthru(false)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.SetPassthru(false)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		log.Debugf("Capture will be stopped after %v", d)

		select {
		case <-time.After(d):
			tc.StopCapture()
		case <-done:
		}
	}()

	for msg := range tc.Traffic {
		err = tc.processMessage(msg)

		if err != nil {
			log.Errorf("There was an issue writing the captured message to disk, stopping capture: %v", err)
			tc.StopCapture()
		}
	}

	n, err := tc.writeState()
	if err != nil {
		log.Warnf("There was an issue writing the capture state, capture file may be corrupt: %v", err)
	} else {
		log.Warnf("Wrote %d bytes for capture tagger state", n)
	}

	err = tc.writer.Flush()
	if err != nil {
		log.Errorf("There was an error flushing the underlying writer while stopping the capture: %v", err)
	}

	if tc.zWriter != nil {
		err = tc.zWriter.Close()
		if err != nil {
			log.Errorf("There was an error closing the underlying zstd writer while stopping the capture: %v", err)
		}
	}

	tc.Lock()
	defer tc.Unlock()
	tc.ongoing = false
}

// StopCapture stops the ongoing capture if in process.
func (tc *TrafficCaptureWriter) StopCapture() {
	tc.Lock()
	defer tc.Unlock()

	if !tc.ongoing {
		return
	}

	if tc.accepting {
		close(tc.Traffic)
		tc.accepting = false
	}

	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.SetPassthru(true)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.SetPassthru(true)
	}

	log.Debug("Capture was stopped")
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *TrafficCaptureWriter) Enqueue(msg *CaptureBuffer) bool {
	tc.RLock()
	defer tc.RUnlock()

	if tc.accepting {
		tc.Traffic <- msg
		return true
	}

	return false
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
	tc.RLock()
	defer tc.RUnlock()

	return tc.ongoing
}

// writeHeader writes the .dog file format header to the capture file.
func (tc *TrafficCaptureWriter) writeHeader() error {
	return WriteHeader(tc.writer)
}

// writeState writes the tagger state to the capture file.
func (tc *TrafficCaptureWriter) writeState() (int, error) {

	pbState := &pb.TaggerState{
		State:  make(map[string]*pb.Entity),
		PidMap: tc.taggerState,
	}

	// iterate entities
	for _, id := range tc.taggerState {
		entity, err := tagger.GetEntity(id)
		if err != nil {
			log.Warnf("There was no entity for container id: %v present in the tagger", entity)
			continue
		}

		entityID, err := utils.Tagger2PbEntityID(entity.ID)
		if err != nil {
			log.Warnf("unable to compute valid EntityID for %v", id)
			continue
		}

		entry := pb.Entity{
			// TODO: Hash:               entity.Hash,
			Id:                          entityID,
			HighCardinalityTags:         entity.HighCardinalityTags,
			OrchestratorCardinalityTags: entity.OrchestratorCardinalityTags,
			LowCardinalityTags:          entity.LowCardinalityTags,
			StandardTags:                entity.StandardTags,
		}
		pbState.State[id] = &entry
	}

	log.Debugf("Going to write STATE: %#v", pbState)

	s, err := proto.Marshal(pbState)
	if err != nil {
		return 0, err
	}

	// Record State Separator
	if n, err := tc.writer.Write([]byte{0, 0, 0, 0}); err != nil {
		return n, err
	}

	// Record State
	n, err := tc.writer.Write(s)

	// Record size
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(s)))

	if n, err := tc.writer.Write(buf); err != nil {
		return n, err
	}

	// n + 4 bytes for separator + 4 bytes for state size
	return n + 8, err
}

// writeNext writes the next CaptureBuffer after serializing it to a protobuf format.
// Continuing writes after an error calling this function would result in a corrupted file
func (tc *TrafficCaptureWriter) writeNext(msg *CaptureBuffer) error {
	buff, err := proto.Marshal(&msg.Pb)
	if err != nil {
		return err
	}

	_, err = tc.Write(buff)
	return err
}

// Write writes the byte slice argument to file.
func (tc *TrafficCaptureWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(len(p)))

	// Record size
	if n, err := tc.writer.Write(buf); err != nil {
		return n, err
	}

	// Record
	n, err := tc.writer.Write(p)

	return n + 4, err
}
