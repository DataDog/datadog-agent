package metric

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
	flowmessage "github.com/netsampler/goflow2/pb"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Driver desc
type Driver struct {
	fileDestination string
	lineSeparator   string
	w               io.Writer
	file            *os.File
	Lock            *sync.RWMutex
	q               chan bool
	MetricChan      chan []metrics.MetricSample
}

// Prepare desc
func (d *Driver) Prepare() error {
	d.fileDestination = ""
	d.lineSeparator = "\n"
	return nil
}

func (d *Driver) openFile() error {
	file, err := os.OpenFile(d.fileDestination, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	d.file = file
	d.w = d.file
	return err
}

// Init desc
func (d *Driver) Init(context.Context) error {
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
					err := d.openFile()
					if err != nil {
						return
					}
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

// Send desc
func (d *Driver) Send(key, data []byte) error {
	//d.Lock.RLock()
	//w := d.w
	//d.Lock.RUnlock()
	log.Warn("Send metric")

	buf := proto.NewBuffer(data)

	msg := new(flowmessage.FlowMessage)
	err := buf.DecodeMessage(msg)

	log.Warnf("msg.SrcAddr: %v", msg.SrcAddr)

	//_, err := fmt.Fprint(w, string(data)+d.lineSeparator)

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

// Close desc
func (d *Driver) Close(context.Context) error {
	if d.fileDestination != "" {
		d.Lock.Lock()
		d.file.Close()
		d.Lock.Unlock()
		signal.Ignore(syscall.SIGHUP)
	}
	close(d.q)
	return nil
}
