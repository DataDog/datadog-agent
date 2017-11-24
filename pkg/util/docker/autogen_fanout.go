package docker

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/fanout"
	log "github.com/cihub/seelog"
	"io"
	"sync"
	"time"
)

type eventFanout struct {
	sync.RWMutex
	config    fanout.Config
	dataInput chan *ContainerEvent
	stopChan  chan error
	listeners map[string]*eventOutput
	running   bool
}

func (f *eventFanout) Setup(cfg fanout.Config) (chan<- *ContainerEvent, error) {
	if cfg.WriteTimeout.Nanoseconds() == 0 {
		return nil, errors.New("WriteTimeout must be higher than 0")
	}
	if cfg.OutputBufferSize == 0 {
		return nil, errors.New("OutputBufferSize must be higher than 0")
	}
	if cfg.Name == "" {
		return nil, errors.New("Name can't be empty")
	}
	f.Lock()
	defer f.Unlock()
	f.config = cfg
	f.dataInput = make(chan *ContainerEvent)
	f.stopChan = make(chan error, 1)
	f.listeners = make(map[string]*eventOutput)
	return f.dataInput, nil
}
func (f *eventFanout) StopOnEOF() {
	f.StopOnError(io.EOF)
}
func (f *eventFanout) StopOnError(err error) {
	f.stopChan <- err
}
func (f *eventFanout) Suscribe(name string) (<-chan *ContainerEvent, <-chan error, error) {
	f.Lock()
	defer f.Unlock()
	if _, found := f.listeners[name]; found {
		return nil, nil, fmt.Errorf("listener %s is already suscribed to %s", name, f.config.Name)
	}
	out := &eventOutput{dataOutput: make(chan *ContainerEvent, f.config.OutputBufferSize), errorOutput: make(chan error, 1), writeTimeout: f.config.WriteTimeout}
	f.listeners[name] = out
	if !f.running {
		f.running = true
		go f.dispatch()
	}
	return out.dataOutput, out.errorOutput, nil
}
func (f *eventFanout) Unsuscribe(name string) (bool, error) {
	return f.UnsuscribeWithError(name, io.EOF)
}
func (f *eventFanout) UnsuscribeWithError(name string, err error) (bool, error) {
	f.Lock()
	defer f.Unlock()
	if _, found := f.listeners[name]; !found {
		return false, fmt.Errorf("listener %s is not suscribed to %s", name, f.config.Name)
	}
	f.listeners[name].close(err)
	delete(f.listeners, name)
	if f.running && len(f.listeners) == 0 {
		f.StopOnEOF()
		return true, nil
	}
	return false, nil
}
func (f *eventFanout) dispatch() {
	badListeners := make(map[string]error)
	for {
	TRANSMIT:
		for {
			select {
			case err := <-f.stopChan:
				f.Lock()
				for name, output := range f.listeners {
					output.close(err)
					delete(f.listeners, name)
				}
				f.running = false
				f.Unlock()
				return
			case data := <-f.dataInput:
				f.RLock()
				for name, output := range f.listeners {
					err := output.sendData(data)
					if err != nil {
						badListeners[name] = err
					}
				}
				f.RUnlock()
				break TRANSMIT
			}
		}
		if len(badListeners) == 0 {
			continue
		}
		for name, err := range badListeners {
			log.Infof("forcefully unsuscribing %s from %s: %s", name, f.config.Name, err)
			f.UnsuscribeWithError(name, err)
		}
		badListeners = make(map[string]error)
	}
}

type eventOutput struct {
	dataOutput   chan *ContainerEvent
	errorOutput  chan error
	writeTimeout time.Duration
}

func (o *eventOutput) sendData(data *ContainerEvent) error {
	select {
	case o.dataOutput <- data:
		return nil
	case <-time.After(o.writeTimeout):
		return fanout.ErrTimeout
	}
}
func (o *eventOutput) sendError(err error) error {
	select {
	case o.errorOutput <- err:
		return nil
	case <-time.After(o.writeTimeout):
		return fanout.ErrTimeout
	}
}
func (o *eventOutput) close(err error) {
	o.sendError(err)
	close(o.dataOutput)
	close(o.errorOutput)
}
