package file

import (
	"context"
	"flag"
	"fmt"
	"github.com/netsampler/goflow2/transport"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type FileDriver struct {
	fileDestination string
	lineSeparator   string
	w               io.Writer
	file            *os.File
	lock            *sync.RWMutex
	q               chan bool
}

func (d *FileDriver) Prepare() error {
	flag.StringVar(&d.fileDestination, "transport.file", "", "File/console output (empty for stdout)")
	flag.StringVar(&d.lineSeparator, "transport.file.sep", "\n", "Line separator")
	// idea: add terminal coloring based on key partitioning (if any)
	return nil
}

func (d *FileDriver) openFile() error {
	file, err := os.OpenFile(d.fileDestination, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	d.file = file
	d.w = d.file
	return err
}

func (d *FileDriver) Init(context.Context) error {
	d.q = make(chan bool, 1)

	if d.fileDestination == "" {
		d.w = os.Stdout
	} else {
		var err error

		d.lock.Lock()
		err = d.openFile()
		d.lock.Unlock()
		if err != nil {
			return err
		}

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP)
		go func() {
			for {
				select {
				case <-c:
					d.lock.Lock()
					d.file.Close()
					d.openFile()
					d.lock.Unlock()
					// if there is an error, keeps using the old file
				case <-d.q:
					return
				}
			}
		}()

	}
	return nil
}

func (d *FileDriver) Send(key, data []byte) error {
	d.lock.RLock()
	w := d.w
	d.lock.RUnlock()
	_, err := fmt.Fprint(w, string(data)+d.lineSeparator)
	return err
}

func (d *FileDriver) Close(context.Context) error {
	if d.fileDestination != "" {
		d.lock.Lock()
		d.file.Close()
		d.lock.Unlock()
		signal.Ignore(syscall.SIGHUP)
	}
	close(d.q)
	return nil
}

func init() {
	d := &FileDriver{
		lock: &sync.RWMutex{},
	}
	transport.RegisterTransportDriver("file", d)
}
