// +build linux_bpf

package ebpf

import (
	"fmt"
	"os"

	cbpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	bpflib "github.com/iovisor/gobpf/elf"
	"golang.org/x/sys/unix"
)

type perfMap struct {
	rdr         *perf.Reader
	receiveChan chan []byte
	lostChan    chan uint64
}

func initPerfMap(mod *bpflib.Module, mapName string, receiverChan chan []byte, lostChan chan uint64) (*perfMap, error) {
	mp := mod.Map(mapName)

	// we must duplicate the FD using fcntl(2) because gobpf will not forget the FD.
	fd, err := dupFd(mp.Fd())
	if err != nil {
		return nil, fmt.Errorf("unable to duplicate fd: %s", err)
	}
	cmap, err := cbpf.NewMapFromFD(fd)
	if err != nil {
		return nil, fmt.Errorf("error creating cilium map from fd: %s", err)
	}
	// map will be cloned by perf.Reader, so we can close this one
	defer cmap.Close()

	bufSize := os.Getpagesize() * 8
	rdr, err := perf.NewReader(cmap, bufSize)
	if err != nil {
		return nil, fmt.Errorf("error creating perf map reader: %s", err)
	}

	return &perfMap{
		rdr:         rdr,
		receiveChan: receiverChan,
		lostChan:    lostChan,
	}, nil
}

func dupFd(fd int) (int, error) {
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return 0, fmt.Errorf("can't dup fd: %s", err)
	}
	return dup, nil
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
