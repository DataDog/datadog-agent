package bcc

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/iovisor/gobpf/bcc"
)

var DefaultBufferLength = 100

type PerfMap struct {
	*types.PerfMap

	dataChan   chan []byte
	bccPerfMap *bcc.PerfMap
	wg         sync.WaitGroup
}

func (m *PerfMap) Start() error {
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()

		m.bccPerfMap.Start()
		defer m.bccPerfMap.Stop()

		for data := range m.dataChan {
			m.Handler(data)
		}
	}()

	return nil
}

func (m *PerfMap) Stop() {
	if m.dataChan != nil {
		close(m.dataChan)
	}
	m.wg.Wait()
}
