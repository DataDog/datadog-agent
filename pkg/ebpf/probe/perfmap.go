package probe

import (
	"fmt"
	"sync"

	"github.com/iovisor/gobpf/bcc"
)

var DefaultBufferLength = 100

type Handler func([]byte)

type PerfMap struct {
	Name         string
	BufferLength int
	Handler      Handler

	dataChan   chan []byte
	bccTable   *bcc.Table
	bccPerfMap *bcc.PerfMap
	wg         sync.WaitGroup
}

func (m *PerfMap) Start(module *Module) error {
	if m.Handler == nil {
		return fmt.Errorf("no handler specified for perfmap %s", m.Name)
	}

	bufferLength := m.BufferLength
	if bufferLength == 0 {
		bufferLength = DefaultBufferLength
	}

	var err error
	m.dataChan = make(chan []byte, bufferLength)
	m.bccTable = bcc.NewTable(module.TableId(m.Name), module.Module)
	m.bccPerfMap, err = bcc.InitPerfMap(m.bccTable, m.dataChan)
	if err != nil {
		return fmt.Errorf("failed to start perf map: %s", err)
	}

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
