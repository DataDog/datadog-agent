// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// servicePort is a tiny port struct used by fakeService so tests don't depend
// on the workloadmeta package internals beyond the public ContainerPort.
type servicePort struct {
	Name string
	Port int
}

// fakeService is a minimal ServiceInfo implementation for the
// discoverer package's unit tests.
type fakeService struct {
	id       string
	hosts    map[string]string
	hostsErr error
	ports    []servicePort
	portsErr error
}

var _ ServiceInfo = (*fakeService)(nil)

func (f *fakeService) GetServiceID() string { return f.id }
func (f *fakeService) GetHosts() (map[string]string, error) {
	return f.hosts, f.hostsErr
}
func (f *fakeService) GetPorts() ([]workloadmeta.ContainerPort, error) {
	if f.portsErr != nil {
		return nil, f.portsErr
	}
	out := make([]workloadmeta.ContainerPort, 0, len(f.ports))
	for _, p := range f.ports {
		out = append(out, workloadmeta.ContainerPort{Name: p.Name, Port: p.Port})
	}
	return out, nil
}

// fixedLookup is a ServiceLookup mock implementation.
type fixedLookup struct {
	mu       sync.Mutex
	services map[string]ServiceInfo
}

func (l *fixedLookup) LookupService(svcID string) (ServiceInfo, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.services == nil {
		return nil, false
	}
	svc, ok := l.services[svcID]
	return svc, ok
}

// remove drops the given svcID from the lookup, simulating a service
// deletion observed by the worker on its next pop.
func (l *fixedLookup) remove(svcID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.services, svcID)
}
