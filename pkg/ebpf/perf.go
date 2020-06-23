// +build linux_bpf

package ebpf

import (
	"fmt"
	cbpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	bpflib "github.com/iovisor/gobpf/elf"
)

type perfMap struct {
	rdr         *perf.Reader
	receiveChan chan []byte
	lostChan    chan uint64
}

func initPerfMap(mod *bpflib.Module, mapName string, receiverChan chan []byte, lostChan chan uint64) (*perfMap, error) {
	mp := mod.Map(mapName)

	// cbpf will assume ownership of the map FD at this point, but gobpf does not have a way to "forget" about it.
	// In practice this is not an issue, because it will only result in the close(2) syscall being called on the FD twice.
	// This does result in an error returned from the syscall, but those errors are currently ignored by our code.
	cmap, err := cbpf.NewMapFromFD(mp.Fd())
	if err != nil {
		return nil, fmt.Errorf("error creating cilium map from fd: %s", err)
	}

	rdr, err := perf.NewReader(cmap, 4096)
	if err != nil {
		return nil, fmt.Errorf("error creating perf map reader: %s", err)
	}

	return &perfMap{
		rdr:         rdr,
		receiveChan: receiverChan,
		lostChan:    lostChan,
	}, nil
}

func (pm *perfMap) PollStart() {
	go func() {
		defer func() {
			close(pm.receiveChan)
			close(pm.lostChan)
		}()

		for {
			rec, err := pm.rdr.Read()
			if err != nil {
				if perf.IsClosed(err) {
					break
				}
				continue
			}
			if rec.RawSample != nil {
				pm.receiveChan <- rec.RawSample
			}
			if rec.LostSamples > 0 {
				pm.lostChan <- rec.LostSamples
			}
		}
	}()
}

func (pm *perfMap) PollStop() {
	// this will interrupt an in-progress Read()
	_ = pm.rdr.Close()
}
