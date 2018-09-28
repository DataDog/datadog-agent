// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listeners

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/procdiscovery"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessPollInterval defines how often we should query for running processes
var ProcessPollInterval = 10 * time.Second

// ProcessListener implements the ServiceListener interface.
// It regularly polls running processes and report them to Auto Discovery
// It also holds a cache of services that the ConfigResolver can query to
// match templates against.
type ProcessListener struct {
	services   map[int32]Service
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	health     *health.Handle
	m          sync.RWMutex
}

// ProcessService implements and store results from the Service interface for the Process listener
type ProcessService struct {
	sync.RWMutex
	adIdentifiers []string
	ports         []ContainerPort
	pid           int
	hostname      string
	name          string
	creationTime  integration.CreationTime
	unixSockets   []string
}

func init() {
	Register("process", NewProcessListener)
}

// NewProcessListener creates a process listener
func NewProcessListener() (ServiceListener, error) {
	return &ProcessListener{
		services: make(map[int32]Service),
		stop:     make(chan bool),
		health:   health.Register("ad-processlistener"),
	}, nil
}

// Listen listens on the hosts processes and report found services
func (l *ProcessListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	// poll for already running processes
	l.pollProcesses(integration.Before)

	ticker := time.NewTicker(ProcessPollInterval)

	go func() {
		for {
			select {
			case <-l.stop:
				ticker.Stop()
				l.health.Deregister()
				return
			case <-l.health.C:
			case _ = <-ticker.C:
				l.pollProcesses(integration.After)
			}
		}
	}()
}

// Stop queues a shutdown of ProcessListener
func (l *ProcessListener) Stop() {
	l.stop <- true
}

// pollProcesses requests the running processes and tries to find a service linked
// to them and figure out if the ConfigResolver could be interested to inspect it
func (l *ProcessListener) pollProcesses(creationTime integration.CreationTime) {
	discovered, err := procdiscovery.DiscoverIntegrations()
	if err != nil {
		log.Errorf("process poller error while discovery: %v", err)
	}

	pids := map[int32]procdiscovery.IntegrationProcess{}

	for _, procs := range discovered {
		for _, proc := range procs {
			pids[proc.PID] = proc
		}
	}

	l.m.Lock()
	for pid := range l.services {
		if _, ok := pids[pid]; ok {
			// Process is already registered in services list remove it from the list of pids
			delete(pids, pid)
		} else {
			// Given process is no more running, remove it from the list of services
			l.m.Unlock()
			l.removeService(pid)
			l.m.Lock()
		}
	}
	l.m.Unlock()

	// loop on left services and create them
	for _, proc := range pids {
		l.createService(proc, creationTime)
	}
}

// createService takes a procdiscovery.Process, create a service for it in its cache
// and tells the ConfigResolver that this service started.
func (l *ProcessListener) createService(proc procdiscovery.IntegrationProcess, creationTime integration.CreationTime) {
	hostname := "127.0.0.1"

	svc := &ProcessService{
		adIdentifiers: []string{strings.ToLower(proc.Name), strings.ToLower(proc.DisplayName), strings.ToLower(proc.Cmd)},
		pid:           int(proc.PID),
		hostname:      hostname,
		name:          proc.Name,
		ports:         []ContainerPort{},
		creationTime:  creationTime,
	}

	ports, err := getProcessPorts(proc.PID)
	if err != nil {
		log.Errorf("Couldn't retrieve connections for process (%s, pid: %v): %s", proc.Cmd, proc.PID, err)
		return
	}

	sockets, err := getProcessUnixSockets(proc.PID)
	if err != nil {
		log.Errorf("Couldn't retrieve unix sockets for process (%s, pid: %v): %s", proc.Cmd, proc.PID, err)
		return
	}
	svc.unixSockets = sockets

	for _, port := range ports {
		svc.ports = append(svc.ports, ContainerPort{Port: port})
	}

	l.m.Lock()
	l.services[proc.PID] = svc
	l.m.Unlock()

	l.newService <- svc
}

// removeService takes a process pid, removes the related service from its cache
// and tells the ConfigResolver that this service stopped.
func (l *ProcessListener) removeService(pid int32) {
	l.m.RLock()
	svc, ok := l.services[pid]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, pid)
		l.m.Unlock()

		l.delService <- svc
	} else {
		log.Debugf("Process (pid: %v) not found, not removing", pid)
	}
}

// GetEntity returns the unique entity name linked to that service
func (s *ProcessService) GetEntity() string {
	return fmt.Sprintf("process://%s:%v", s.name, s.pid)
}

// GetADIdentifiers returns a set of AD identifiers for a process.
// These id are sorted to reflect the priority we want the ConfigResolver to
// use when matching a template.
//
// The order is:
//   1. matched integration name
//   2. matched integration display name
// 	 3. matched integration process' command line
func (s *ProcessService) GetADIdentifiers() ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the process' hosts
func (s *ProcessService) GetHosts() (map[string]string, error) {
	return map[string]string{"host": s.hostname}, nil
}

// GetPorts returns the process' ports
func (s *ProcessService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *ProcessService) GetTags() ([]string, error) {
	tags, err := tagger.Tag(s.GetEntity(), tagger.IsFullCardinality())
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}

// GetPid returns the process' pid
func (s *ProcessService) GetPid() (int, error) {
	return s.pid, nil
}

// GetHostname returns the process' hostname
func (s *ProcessService) GetHostname() (string, error) {
	return s.hostname, nil
}

// GetCreationTime returns the creation time of the service compare to the agent start.
func (s *ProcessService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// GetUnixSockets returns the unix sockets used by the process
func (s *ProcessService) GetUnixSockets() ([]string, error) {
	return s.unixSockets, nil
}
