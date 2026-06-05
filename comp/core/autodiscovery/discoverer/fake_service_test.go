// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// servicePort is a tiny port struct used by fakeService so tests don't depend
// on the workloadmeta package internals beyond the public ContainerPort.
type servicePort struct {
	Name string
	Port int
}

// fakeService is a minimal listeners.Service implementation for the
// discoverer package's unit tests. The real config manager hands the worker
// genuine workloadmeta-backed services, but the discoverer itself only cares
// about the Service interface — so the test double is fine.
type fakeService struct {
	id       string
	adIDs    []string
	hosts    map[string]string
	hostsErr error
	ports    []servicePort
	portsErr error
}

var _ listeners.Service = (*fakeService)(nil)

func (f *fakeService) Equal(o listeners.Service) bool                    { _, ok := o.(*fakeService); return ok }
func (f *fakeService) GetServiceID() string                              { return f.id }
func (f *fakeService) GetADIdentifiers() []string                        { return f.adIDs }
func (f *fakeService) GetHosts() (map[string]string, error)              { return f.hosts, f.hostsErr }
func (f *fakeService) GetTags() ([]string, error)                        { return nil, nil }
func (f *fakeService) GetTagsWithCardinality(_ string) ([]string, error) { return nil, nil }
func (f *fakeService) GetPid() (int, error)                              { return 0, nil }
func (f *fakeService) GetHostname() (string, error)                      { return "", nil }
func (f *fakeService) IsReady() bool                                     { return true }
func (f *fakeService) HasFilter(_ workloadfilter.Scope) bool             { return false }
func (f *fakeService) GetExtraConfig(_ string) (string, error)           { return "", nil }
func (f *fakeService) GetImageName() string                              { return "" }
func (f *fakeService) FilterTemplates(_ map[string]integration.Config)   {}
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
	services map[string]listeners.Service
}

func (l *fixedLookup) LookupService(svcID string) (listeners.Service, bool) {
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
