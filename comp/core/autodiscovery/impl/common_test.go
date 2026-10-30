// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

type dummyService struct {
	ID              string
	ADIdentifiers   []string
	Hosts           map[string]string
	Ports           []workloadmeta.ContainerPort
	Pid             int
	Hostname        string
	Tags            []string
	filterTemplates func(map[string]integration.Config)
}

// Equal returns whether the two dummyService are equal
func (s *dummyService) Equal(o listeners.Service) bool {
	return reflect.DeepEqual(s, o)
}

// GetServiceID returns the service entity name
func (s *dummyService) GetServiceID() string {
	return s.ID
}

// GetADIdentifiers returns dummy identifiers
func (s *dummyService) GetADIdentifiers() []string {
	return s.ADIdentifiers
}

// GetHosts returns dummy hosts
func (s *dummyService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns dummy ports
func (s *dummyService) GetPorts() ([]workloadmeta.ContainerPort, error) {
	return s.Ports, nil
}

// GetTags returns the tags for this service
func (s *dummyService) GetTags() ([]string, error) {
	return s.Tags, nil
}

// GetTagsWithCardinality returns the tags for this service
func (s *dummyService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetPid return a dummy pid
func (s *dummyService) GetPid() (int, error) {
	return s.Pid, nil
}

// GetHostname return a dummy hostname
func (s *dummyService) GetHostname() (string, error) {
	return s.Hostname, nil
}

// IsReady returns if the service is ready
func (s *dummyService) IsReady() bool {
	return true
}

// HasFilter returns false
func (s *dummyService) HasFilter(_ workloadfilter.Scope) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *dummyService) GetExtraConfig(_ string) (string, error) {
	return "", nil
}

// FilterTemplates calls filterTemplates, if not nil
func (s *dummyService) FilterTemplates(configs map[string]integration.Config) {
	if s.filterTemplates != nil {
		(s.filterTemplates)(configs)
	}
}

// GetImageName does nothing
func (s *dummyService) GetImageName() string {
	return ""
}

// configChangeRecorder observes received change publications.
type configChangeRecorder struct {
	mu          sync.Mutex
	changes     []integration.ConfigChanges
	lastChanges integration.ConfigChanges
}

func (r *configChangeRecorder) record(changes integration.ConfigChanges, _ map[checkid.ID]checkid.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastChanges = changes
	if len(changes.Schedule)+len(changes.Unschedule) == 0 {
		return
	}
	r.changes = append(r.changes, changes)
}

func (r *configChangeRecorder) snapshot() []integration.ConfigChanges {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.changes)
}

func (r *configChangeRecorder) wait(t *testing.T) integration.ConfigChanges {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.snapshot()) > 0 }, 2*time.Second, time.Millisecond,
		"timed out waiting for published changes")
	r.mu.Lock()
	defer r.mu.Unlock()
	changes := r.changes[0]
	r.changes = r.changes[1:]
	return changes
}

func (r *configChangeRecorder) last() integration.ConfigChanges {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastChanges
}
