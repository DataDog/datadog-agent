package gobpf

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	bpflib "github.com/iovisor/gobpf/elf"
)

var (
	DefaultBufferLength      = 256
	DefaultClosedChannelSize = 1000
	DefaultLostEventSize     = 10
)

type PerfMap struct {
	*bpflib.PerfMap

	handler       func([]byte)
	eventChannel  chan []byte
	lostChannel   chan uint64
	receivedCount int64
	lostCount     int64
}

func (p *PerfMap) Start() error {
	p.PollStart()

	go func() {
		for {
			select {
			case data, ok := <-p.eventChannel:
				if !ok {
					log.Infof("Exiting closed connections polling")
					return
				}
				atomic.AddInt64(&p.receivedCount, 1)

				p.handler(data)
			case lostCount, ok := <-p.lostChannel:
				if !ok {
					return
				}
				atomic.AddInt64(&p.lostCount, int64(lostCount))
			}
		}
	}()

	return nil
}

func (p *PerfMap) Stop() {
	p.PollStop()
}
