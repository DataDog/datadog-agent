// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"sync"

	manager "github.com/DataDog/ebpf-manager"
)

type PerfHandler struct {
	DataChannel chan *DataEvent
	LostChannel chan uint64
	once        sync.Once
	closed      bool
}

type DataEvent struct {
	CPU  int
	Data []byte
}

func NewPerfHandler(dataChannelSize int) *PerfHandler {
	return &PerfHandler{
		DataChannel: make(chan *DataEvent, dataChannelSize),
		LostChannel: make(chan uint64, 10),
	}
}

func (c *PerfHandler) DataHandler(CPU int, data []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.DataChannel <- &DataEvent{CPU, data}
}

func (c *PerfHandler) LostHandler(CPU int, lostCount uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	if c.closed {
		return
	}
	c.LostChannel <- lostCount
}

func (c *PerfHandler) Stop() {
	c.once.Do(func() {
		c.closed = true
		close(c.DataChannel)
		close(c.LostChannel)
	})
}
