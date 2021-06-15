// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replay

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	// Refactor relevant bits
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/afero"

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

// CapPool is a pool of CaptureBuffer
var CapPool = sync.Pool{
	New: func() interface{} {
		return new(CaptureBuffer)
	},
}

// for testing purposes, modify atomically
var inMemoryFs int64

// TrafficCaptureWriter allows writing dogstatsd traffic to a file.
type TrafficCaptureWriter struct {
	File     *os.File
	testFile afero.File
	writer   *bufio.Writer
	Traffic  chan *CaptureBuffer
	Location string
	shutdown chan struct{}
	ongoing  bool

	sharedPacketPoolManager *packets.PoolManager
	oobPacketPoolManager    *packets.PoolManager

	taggerState map[int32]string

	sync.RWMutex
}

// NewTrafficCaptureWriter creates a TrafficCaptureWriter instance.
func NewTrafficCaptureWriter(l string, depth int) *TrafficCaptureWriter {

	return &TrafficCaptureWriter{
		Location:    l,
		Traffic:     make(chan *CaptureBuffer, depth),
		taggerState: make(map[int32]string),
	}
}

// Path returns the path to file where the traffic capture will be written.
func (tc *TrafficCaptureWriter) Path() (string, error) {
	tc.RLock()
	defer tc.RUnlock()

	if tc.File == nil {
		return "", fmt.Errorf("No file set in writer")
	}

	return filepath.Abs(tc.File.Name())
}

// ProcessMessage receives a capture buffer and writes it to disk while also tracking
// the PID map to be persisted to the taggerState. Should not normally be called directly.
func (tc *TrafficCaptureWriter) ProcessMessage(msg *CaptureBuffer) error {

	tc.Lock()

	err := tc.WriteNext(msg)
	if err != nil {
		tc.Unlock()
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
	tc.Unlock()

	return nil
}

// Capture start the traffic capture and writes the packets to file for the specified duration.
func (tc *TrafficCaptureWriter) Capture(d time.Duration) {

	log.Debug("Starting capture...")

	var err error

	tc.Lock()
	p := path.Join(tc.Location, fmt.Sprintf(fileTemplate, time.Now().Unix()))
	if err = os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		log.Errorf("There was an issue writing the expected location: %v ", err)
		tc.Unlock()
		return
	}

	// inMemoryFS is used for testing purposes
	if atomic.LoadInt64(&inMemoryFs) > 0 {
		appFS := afero.NewMemMapFs()
		err := appFS.MkdirAll(tc.Location, 0755)
		if err != nil {
			log.Errorf("There was an issue starting the capture: %v ", err)

			tc.Unlock()
			return
		}

		fp, err := appFS.Create(p)
		if err != nil {
			log.Errorf("There was an issue starting the capture: %v ", err)

			tc.Unlock()
			return
		}

		tc.testFile = fp
		tc.writer = bufio.NewWriter(tc.testFile)
	} else {
		fp, err := os.Create(p)
		if err != nil {
			log.Errorf("There was an issue starting the capture: %v ", err)

			tc.Unlock()
			return
		}
		tc.File = fp
		tc.writer = bufio.NewWriter(tc.File)
	}

	tc.shutdown = make(chan struct{})
	tc.ongoing = true

	err = tc.WriteHeader()
	if err != nil {
		log.Errorf("There was an issue writing the capture file header: %v ", err)
		tc.Unlock()

		return
	}

	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.SetPassthru(false)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.SetPassthru(false)
	}
	tc.Unlock()

	go func() {
		log.Debugf("Capture will be stopped after %v", d)

		<-time.After(d)
		tc.StopCapture()
	}()

process:
	for {
		select {
		case msg := <-tc.Traffic:
			err = tc.ProcessMessage(msg)

			if err != nil {
				log.Errorf("There was an issue writing the captured message to disk, stopping capture: %v", err)
				tc.StopCapture()
			}
		case <-tc.shutdown:
			log.Debug("Capture shutting down")
			tc.Lock()
			tc.shutdown = nil
			tc.Unlock()

			break process
		}
	}

	// write any packets remaining in the channel.
cleanup:
	for {
		select {
		case msg := <-tc.Traffic:
			err = tc.ProcessMessage(msg)

			if err != nil {
				log.Errorf("There was an issue writing the captured message to disk, the message will be dropped: %v", err)
			}
		default:
			break cleanup
		}
	}

	n, err := tc.WriteState()
	if err != nil {
		log.Warnf("There was an issue writing the capture state, capture file may be corrupt: %v", err)
	} else {
		log.Warnf("Wrote %d bytes for capture tagger state", n)
	}

	tc.Lock()
	defer tc.Unlock()
	err = tc.writer.Flush()
	if err != nil {
		log.Errorf("There was an error flushing the underlying writer while stopping the capture: %v", err)
	}

	tc.File.Close()
	tc.ongoing = false

}

// StopCapture stops the ongoing capture if in process.
func (tc *TrafficCaptureWriter) StopCapture() {
	tc.Lock()
	defer tc.Unlock()

	if !tc.ongoing {
		return
	}

	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.SetPassthru(true)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.SetPassthru(true)
	}

	if tc.shutdown != nil {
		close(tc.shutdown)
	}

	log.Debug("Capture was stopped")
}

// Enqueue enqueues a capture buffer so it's written to file.
func (tc *TrafficCaptureWriter) Enqueue(msg *CaptureBuffer) bool {
	qd := false

	if tc.IsOngoing() {
		tc.Traffic <- msg
		qd = true
	}

	return qd
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

// WriteHeader writes the .dog file format header to the capture file.
func (tc *TrafficCaptureWriter) WriteHeader() error {
	return WriteHeader(tc.writer)
}

// WriteState writes the tagger state to the capture file.
func (tc *TrafficCaptureWriter) WriteState() (int, error) {

	pbState := pb.TaggerState{
		State:  make(map[string]*pb.Entity),
		PidMap: tc.taggerState,
	}

	// iterate entities
	tc.RLock()
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
	tc.RUnlock()

	log.Debugf("Going to write STATE: %v", pbState)

	s, err := proto.Marshal(&pbState)
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

// WriteNext writes the next CaptureBuffer after serializing it to a protobuf format.
// Continuing writes after an error calling this function would result in a corrupted file
func (tc *TrafficCaptureWriter) WriteNext(msg *CaptureBuffer) error {
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
