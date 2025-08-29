package servicemonitor

import (
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Store struct {
	mu                     sync.RWMutex
	datadogServiceMonitors map[string]DatadogServiceMonitor
	lmu                    sync.RWMutex
	listeners              []chan []DatadogServiceMonitor
}

func NewStore() *Store {
	return &Store{
		mu:                     sync.RWMutex{},
		datadogServiceMonitors: make(map[string]DatadogServiceMonitor),
		lmu:                    sync.RWMutex{},
		listeners:              make([]chan []DatadogServiceMonitor, 0),
	}
}

func (s *Store) SetDatadogServiceMonitor(datadogServiceMonitor DatadogServiceMonitor) {
	log.Infof("Setting service monitor %s in store", datadogServiceMonitor.Name)
	s.mu.Lock()
	s.datadogServiceMonitors[datadogServiceMonitor.Name] = datadogServiceMonitor
	s.mu.Unlock()
	s.notifyListeners()
}

func (s *Store) DeleteDatadogServiceMonitor(name string) {
	log.Infof("Deleting service monitor %s from store", name)
	s.mu.Lock()
	delete(s.datadogServiceMonitors, name)
	s.mu.Unlock()
	s.notifyListeners()
}

func (s *Store) AddListener(listener chan []DatadogServiceMonitor) {
	log.Infof("Adding listener to service monitor store")
	s.lmu.Lock()
	s.listeners = append(s.listeners, listener)
	s.lmu.Unlock()
}

func (s *Store) GetDatadogServiceMonitors() []DatadogServiceMonitor {
	return s.convertToList()
}

func (s *Store) notifyListeners() {
	log.Infof("Notifying listeners of service monitor store")
	monitors := s.convertToList()

	s.lmu.RLock()
	defer s.lmu.RUnlock()
	for _, listener := range s.listeners {
		// prevent blocking the listener
		select {
		case listener <- monitors:
		default:
		}
	}
}

func (s *Store) convertToList() []DatadogServiceMonitor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	datadogServiceMonitors := make([]DatadogServiceMonitor, 0, len(s.datadogServiceMonitors))
	for _, datadogServiceMonitor := range s.datadogServiceMonitors {
		datadogServiceMonitors = append(datadogServiceMonitors, datadogServiceMonitor)
	}

	// Sort by priority
	sort.Slice(datadogServiceMonitors, func(i, j int) bool {
		return datadogServiceMonitors[i].Spec.Priority < datadogServiceMonitors[j].Spec.Priority
	})

	return datadogServiceMonitors
}
