// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package replayimpl

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	// Refactor relevant bits
	"github.com/DataDog/zstd"
	"github.com/benbjohnson/clock"
	"github.com/spf13/afero"

	"google.golang.org/protobuf/proto"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerproto "github.com/DataDog/datadog-agent/comp/core/tagger/proto"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	fileTemplate = "datadog-capture-%d"
)

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

// TrafficCaptureWriter allows writing dogstatsd traffic to a file.
type TrafficCaptureWriter struct {
	depth                   int
	tagger                  tagger.Component
	clock                   clock.Clock
	sharedPacketPoolManager *packets.PoolManager[packets.Packet]
	oobPacketPoolManager    *packets.PoolManager[[]byte]

	// Guards reserving/releasing session, session.stopped, and admission to
	// session.enqueuers. session is atomic so per-packet IsOngoing needs no lock.
	sync.Mutex
	session atomic.Pointer[captureSession]
}

// captureSession owns one capture's queue, stop signal, and output state.
// Only the capture goroutine writes the output and closes traffic and done.
type captureSession struct {
	traffic   chan *replay.CaptureBuffer
	stop      chan struct{}
	done      chan struct{}
	stopped   bool
	enqueuers sync.WaitGroup

	zWriter                 *zstd.Writer
	writer                  *bufio.Writer
	taggerState             map[int32]string
	tagger                  tagger.Component
	sharedPacketPoolManager *packets.PoolManager[packets.Packet]
	oobPacketPoolManager    *packets.PoolManager[[]byte]
}

// NewTrafficCaptureWriter creates a TrafficCaptureWriter instance.
func NewTrafficCaptureWriter(depth int, tagger tagger.Component, clk clock.Clock) *TrafficCaptureWriter {
	return &TrafficCaptureWriter{depth: depth, tagger: tagger, clock: clk}
}

func (tc *captureSession) retain(msg *replay.CaptureBuffer) {
	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.Retain(msg.Buff)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.Retain(msg.Oob)
	}
}

func (tc *captureSession) release(msg *replay.CaptureBuffer) {
	if tc.sharedPacketPoolManager != nil {
		tc.sharedPacketPoolManager.Put(msg.Buff)
	}
	if tc.oobPacketPoolManager != nil {
		tc.oobPacketPoolManager.Put(msg.Oob)
	}
}

// processMessage releases capture's references even if serialization or writing fails.
func (tc *captureSession) processMessage(msg *replay.CaptureBuffer) error {
	defer tc.release(msg)
	if err := tc.writeNext(msg); err != nil {
		return err
	}
	if msg.ContainerID != "" {
		tc.taggerState[msg.Pid] = msg.ContainerID
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

// startCapture reserves a session before starting its goroutine. On error the
// caller still owns target; on success the session closes it before completing.
func (tc *TrafficCaptureWriter) startCapture(target io.WriteCloser, d time.Duration, compressed bool) (*captureSession, error) {
	tc.Lock()
	defer tc.Unlock()
	if tc.session.Load() != nil {
		return nil, errors.New("capture is already running")
	}
	s := &captureSession{
		traffic:                 make(chan *replay.CaptureBuffer, tc.depth),
		stop:                    make(chan struct{}),
		done:                    make(chan struct{}),
		taggerState:             make(map[int32]string),
		tagger:                  tc.tagger,
		sharedPacketPoolManager: tc.sharedPacketPoolManager,
		oobPacketPoolManager:    tc.oobPacketPoolManager,
	}
	tc.session.Store(s)
	go tc.capture(s, target, d, compressed)
	return s, nil
}

func (tc *TrafficCaptureWriter) capture(s *captureSession, target io.WriteCloser, d time.Duration, compressed bool) {
	defer func() {
		target.Close()
		tc.Lock()
		defer tc.Unlock()
		tc.session.CompareAndSwap(s, nil)
		close(s.done)
	}()

	if compressed {
		s.zWriter = zstd.NewWriter(target)
		s.writer = bufio.NewWriter(s.zWriter)
	} else {
		s.writer = bufio.NewWriter(target)
	}
	// The timer stops this session only, even if its callback races with completion.
	timer := tc.clock.AfterFunc(d, func() { tc.stopSession(s) })
	defer timer.Stop()

	err := s.writeHeader()
	if err == nil {
	capture:
		for {
			select {
			case <-s.stop:
				break capture
			case msg := <-s.traffic:
				if err = s.processMessage(msg); err != nil {
					break capture
				}
			}
		}
	}

	// Wake blocked senders before waiting for them. No more senders can enter
	// after stopSession, so the consumer can then close and drain its own queue.
	tc.stopSession(s)
	s.enqueuers.Wait()
	close(s.traffic)
	for msg := range s.traffic {
		if err == nil {
			err = s.processMessage(msg)
		} else {
			// The file is already corrupt; release queued buffers without writing.
			s.release(msg)
		}
	}
	if err != nil {
		log.Errorf("There was an issue writing the capture: %v", err)
	} else if n, err := s.writeState(d); err != nil {
		log.Warnf("There was an issue writing the capture state, capture file may be corrupt: %v", err)
	} else {
		log.Debugf("Wrote %d bytes for capture tagger state", n)
	}
	if err := s.writer.Flush(); err != nil {
		log.Errorf("There was an error flushing the underlying writer while stopping the capture: %v", err)
	}
	if s.zWriter != nil {
		if err := s.zWriter.Close(); err != nil {
			log.Errorf("There was an error closing the underlying zstd writer while stopping the capture: %v", err)
		}
	}
}

// stopSession never waits for the consumer or enqueuers and is safe on old sessions.
func (tc *TrafficCaptureWriter) stopSession(s *captureSession) {
	tc.Lock()
	defer tc.Unlock()
	tc.stopSessionLocked(s)
}

func (tc *TrafficCaptureWriter) stopSessionLocked(s *captureSession) {
	if s != nil && !s.stopped {
		s.stopped = true
		close(s.stop)
	}
}

// StopCapture stops accepting packets and asks the writer to drain and finish.
// It does not wait for the drain; use StopAndWait when the flush must complete.
func (tc *TrafficCaptureWriter) StopCapture() {
	tc.Lock()
	defer tc.Unlock()
	tc.stopSessionLocked(tc.session.Load())
}

// StopAndWait stops the ongoing capture and waits up to timeout for it to drain
// and flush, so shutdown does not truncate the file. The lock is released before
// waiting: the drain needs Enqueue to stay admissible.
func (tc *TrafficCaptureWriter) StopAndWait(timeout time.Duration) {
	tc.Lock()
	s := tc.session.Load()
	tc.stopSessionLocked(s)
	tc.Unlock()

	if s == nil {
		return
	}
	select {
	case <-s.done:
	case <-tc.clock.After(timeout):
		log.Warnf("dogstatsd capture did not finish flushing within %v, capture file may be truncated", timeout)
	}
}

// Enqueue retains the buffers only for this session. The caller must keep its
// own references until Enqueue returns, whether or not the packet is accepted.
func (tc *TrafficCaptureWriter) Enqueue(msg *replay.CaptureBuffer) bool {
	tc.Lock()
	s := tc.session.Load()
	if s == nil || s.stopped {
		tc.Unlock()
		return false
	}
	s.enqueuers.Add(1)
	tc.Unlock()
	defer s.enqueuers.Done()

	s.retain(msg)
	select {
	case s.traffic <- msg:
		return true
	case <-s.stop:
		s.release(msg)
		return false
	}
}

// RegisterSharedPoolManager registers the shared pool manager with the TrafficCaptureWriter.
func (tc *TrafficCaptureWriter) RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error {
	tc.Lock()
	defer tc.Unlock()
	if tc.session.Load() != nil {
		return errors.New("capture is already running")
	}
	if tc.sharedPacketPoolManager != nil {
		return errors.New("shared packet pool manager already registered with the writer")
	}

	tc.sharedPacketPoolManager = p

	return nil
}

// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCaptureWriter.
func (tc *TrafficCaptureWriter) RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error {
	tc.Lock()
	defer tc.Unlock()
	if tc.session.Load() != nil {
		return errors.New("capture is already running")
	}
	if tc.oobPacketPoolManager != nil {
		return errors.New("OOB Pool Manager already registered with the writer")
	}

	tc.oobPacketPoolManager = p

	return nil
}

// IsOngoing returns whether a capture is ongoing for this TrafficCaptureWriter instance.
// The listeners call this per packet, so it must not take the lock.
func (tc *TrafficCaptureWriter) IsOngoing() bool {
	return tc.session.Load() != nil
}

// writeHeader writes the .dog file format header to the capture file.
func (tc *captureSession) writeHeader() error {
	return WriteHeader(tc.writer)
}

// writeState writes the tagger state to the capture file.
func (tc *captureSession) writeState(duration time.Duration) (int, error) {

	pbState := &pb.TaggerState{
		State:    make(map[string]*pb.Entity),
		PidMap:   tc.taggerState,
		Duration: duration.Milliseconds(),
	}

	// iterate entities
	for _, entityIDStr := range tc.taggerState {
		prefix, id, err := types.ExtractPrefixAndID(entityIDStr)
		if err != nil {
			log.Warnf("Invalid entity id: %q", id)
			continue
		}

		entityID := types.NewEntityID(prefix, id)
		entity, err := tc.tagger.GetEntity(entityID)
		if err != nil {
			log.Warnf("There was no entity for container id: %v present in the tagger", entity)
			continue
		}

		pbEntityID, err := taggerproto.Tagger2PbEntityID(entity.ID)
		if err != nil {
			log.Warnf("unable to compute valid EntityID for %v", id)
			continue
		}

		entry := pb.Entity{
			// TODO: Hash:               entity.Hash,
			Id:                          pbEntityID,
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

// writeNext writes the next replay.CaptureBuffer after serializing it to a protobuf format.
// Continuing writes after an error calling this function would result in a corrupted file
func (tc *captureSession) writeNext(msg *replay.CaptureBuffer) error {
	pb := pb.UnixDogstatsdMsg{
		Timestamp:     msg.Pb.Timestamp,
		PayloadSize:   msg.Pb.PayloadSize,
		Payload:       msg.Pb.Payload,
		Pid:           msg.Pb.Pid,
		AncillarySize: msg.Pb.AncillarySize,
		Ancillary:     msg.Pb.Ancillary,
	}

	buff, err := proto.Marshal(&pb)
	if err != nil {
		return err
	}

	_, err = tc.Write(buff)
	return err
}

// Write writes the byte slice argument to file.
func (tc *captureSession) Write(p []byte) (int, error) {
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
