package metric

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type MetricDriver struct {
	fileDestination string
	lineSeparator   string
	w               io.Writer
	file            *os.File
	Lock            *sync.RWMutex
	q               chan bool
	MetricChan      chan []metrics.MetricSample
}

func (d *MetricDriver) Prepare() error {
	d.fileDestination = ""
	d.lineSeparator = "\n"
	return nil
}

func (d *MetricDriver) openFile() error {
	file, err := os.OpenFile(d.fileDestination, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	d.file = file
	d.w = d.file
	return err
}

func (d *MetricDriver) Init(context.Context) error {
	d.q = make(chan bool, 1)

	if d.fileDestination == "" {
		d.w = os.Stdout
	} else {
		var err error

		d.Lock.Lock()
		err = d.openFile()
		d.Lock.Unlock()
		if err != nil {
			return err
		}

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGHUP)
		go func() {
			for {
				select {
				case <-c:
					d.Lock.Lock()
					d.file.Close()
					d.openFile()
					d.Lock.Unlock()
					// if there is an error, keeps using the old file
				case <-d.q:
					return
				}
			}
		}()

	}
	return nil
}

func (d *MetricDriver) Send(key, data []byte) error {
	d.Lock.RLock()
	w := d.w
	d.Lock.RUnlock()
	log.Warn("Send metric")
	_, err := fmt.Fprint(w, string(data)+d.lineSeparator)

	timestamp := float64(time.Now().UnixNano())
	enhancedMetrics := []metrics.MetricSample{{
		Name:       "dd.test.my_metric",
		Value:      999,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"alexTag1:val1"},
		SampleRate: 1,
		Timestamp:  timestamp,
	}}
	d.MetricChan <- enhancedMetrics
	return err
}

func (d *MetricDriver) Close(context.Context) error {
	if d.fileDestination != "" {
		d.Lock.Lock()
		d.file.Close()
		d.Lock.Unlock()
		signal.Ignore(syscall.SIGHUP)
	}
	close(d.q)
	return nil
}

//func init() {
//	d := &MetricDriver{
//		lock: &sync.RWMutex{},
//	}
//	transport.RegisterTransportDriver("metric", d)
//}
