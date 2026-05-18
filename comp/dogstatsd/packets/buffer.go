// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Buffer is a buffer of packets that will automatically flush to the given
// output channel when it is full or after a configurable duration.
// BatchWriter receives flushed DogStatsD packet batches.
//
// The default implementation writes to a Go channel. Experimental ingress-log
// implementations can provide a direct writer so the packet buffer does not need
// to route batches through an intermediate packet-carrying channel.
type BatchWriter interface {
	Write(Packets)
	Len() int
}

type channelBatchWriter struct {
	ch chan Packets
}

// NewChannelBatchWriter returns a BatchWriter backed by a Go channel.
func NewChannelBatchWriter(ch chan Packets) BatchWriter {
	return channelBatchWriter{ch: ch}
}

func (w channelBatchWriter) Write(ps Packets) {
	w.ch <- ps
}

func (w channelBatchWriter) Len() int {
	return len(w.ch)
}

type Buffer struct {
	listenerID     string
	packets        Packets
	flushTimer     *time.Ticker
	bufferSize     uint
	outputWriter   BatchWriter
	closeChannel   chan struct{}
	doneChannel    chan struct{}
	m              sync.Mutex
	telemetryStore *TelemetryStore
}

// NewBuffer creates a new buffer of packets of specified size
func NewBuffer(bufferSize uint, flushTimer time.Duration, outputChannel chan Packets, listenerID string, telemetryStore *TelemetryStore) *Buffer {
	return NewBufferWithWriter(bufferSize, flushTimer, NewChannelBatchWriter(outputChannel), listenerID, telemetryStore)
}

// NewBufferWithWriter creates a new buffer of packets using the provided writer
// for flushed batches.
func NewBufferWithWriter(bufferSize uint, flushTimer time.Duration, outputWriter BatchWriter, listenerID string, telemetryStore *TelemetryStore) *Buffer {
	pb := &Buffer{
		listenerID:     listenerID,
		bufferSize:     bufferSize,
		flushTimer:     time.NewTicker(flushTimer),
		outputWriter:   outputWriter,
		packets:        make(Packets, 0, bufferSize),
		closeChannel:   make(chan struct{}),
		doneChannel:    make(chan struct{}),
		telemetryStore: telemetryStore,
	}
	go pb.flushLoop()
	return pb
}

func (pb *Buffer) flushLoop() {
	for {
		select {
		case <-pb.flushTimer.C:
			pb.m.Lock()
			pb.flush()
			pb.telemetryStore.tlmBufferFlushedTimer.Inc(pb.listenerID)
			pb.m.Unlock()
		case <-pb.closeChannel:
			pb.m.Lock()
			pb.flush()
			pb.m.Unlock()
			close(pb.doneChannel)
			return
		}
	}
}

// Append appends a packet to the packet buffer and flushes if the buffer size is to be exceeded.
func (pb *Buffer) Append(packet *Packet) {
	pb.m.Lock()
	defer pb.m.Unlock()

	packet.ListenerID = pb.listenerID
	pb.telemetryStore.tlmBufferSizeBytes.Add(float64(packet.SizeInBytes()+packet.DataSizeInBytes()), pb.listenerID)

	pb.packets = append(pb.packets, packet)

	pb.telemetryStore.tlmBufferSize.Set(float64(len(pb.packets)), pb.listenerID)

	if uint(len(pb.packets)) >= pb.bufferSize {
		pb.flush()
		pb.telemetryStore.tlmBufferFlushedFull.Inc(pb.listenerID)
	}
}

func (pb *Buffer) flush() {
	if len(pb.packets) > 0 {
		t1 := time.Now()

		pb.telemetryStore.TelemetryTrackPackets(pb.packets, pb.listenerID)
		pb.telemetryStore.tlmBufferSizeBytes.Add(-float64(pb.packets.SizeInBytes()+pb.packets.DataSizeInBytes()), pb.listenerID)

		pb.outputWriter.Write(pb.packets)
		t2 := time.Now()
		pb.telemetryStore.tlmListenerChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), pb.listenerID)

		pb.packets = make(Packets, 0, pb.bufferSize)
	}
	pb.telemetryStore.tlmBufferSize.Set(float64(len(pb.packets)), pb.listenerID)
	pb.telemetryStore.tlmChannelSize.Set(float64(pb.outputWriter.Len()))
}

// Close closes the packet buffer
func (pb *Buffer) Close() {
	close(pb.closeChannel)

	select {
	case <-pb.doneChannel:
	case <-time.After(time.Second):
		log.Debug("Timeout flushing the dogstatsd buffer on stop")
	}

	if pb.listenerID != "" {
		pb.telemetryStore.tlmBufferSize.Delete(pb.listenerID)
		pb.telemetryStore.tlmChannelSize.Delete(pb.listenerID)
		pb.telemetryStore.tlmBufferFlushedFull.Delete(pb.listenerID)
		pb.telemetryStore.tlmBufferFlushedTimer.Delete(pb.listenerID)
	}
}
