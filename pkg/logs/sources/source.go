// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.uber.org/atomic"
)

// SourceType used for log line parsing logic.
// TODO: remove this logic.
type SourceType string

const (
	// DockerSourceType docker source type
	DockerSourceType SourceType = "docker"
	// KubernetesSourceType kubernetes source type
	KubernetesSourceType SourceType = "kubernetes"
)

// LogSource holds a reference to an integration name and a log configuration, and allows to track errors and
// successful operations on it. Both name and configuration are static for now and determined at creation time.
// Changing the status is designed to be thread safe.
type LogSource struct {
	Name     string
	Config   *config.LogsConfig
	Status   *status.LogStatus
	inputs   map[string]bool
	lock     *sync.Mutex
	Messages *config.Messages
	// sourceType is the type of the source that we are tailing whereas Config.Type is the type of the tailer
	// that reads log lines for this source. E.g, a sourceType == containerd and Config.Type == file means that
	// the agent is tailing a file to read logs of a containerd container
	sourceType SourceType
	info       map[string]status.InfoProvider
	// In the case that the source is overridden, keep a reference to the parent for bubbling up information about the child
	ParentSource *LogSource
	// LatencyStats tracks internal stats on the time spent by messages from this source in a processing pipeline, i.e.
	// the duration between when a message is decoded by the tailer/listener/decoder and when the message is handled by a sender
	LatencyStats     *util.StatsTracker
	BytesRead        *atomic.Int64
	hiddenFromStatus bool
}

// NewLogSource creates a new log source.
func NewLogSource(name string, cfg *config.LogsConfig) *LogSource {
	return &LogSource{
		Name:             name,
		Config:           cfg,
		Status:           status.NewLogStatus(),
		inputs:           make(map[string]bool),
		lock:             &sync.Mutex{},
		Messages:         config.NewMessages(),
		BytesRead:        atomic.NewInt64(0),
		info:             make(map[string]status.InfoProvider),
		LatencyStats:     util.NewStatsTracker(time.Hour*24, time.Hour),
		hiddenFromStatus: false,
	}
}

// AddInput registers an input as being handled by this source.
func (s *LogSource) AddInput(input string) {
	s.lock.Lock()
	s.inputs[input] = true
	s.lock.Unlock()
}

// RemoveInput removes an input from this source.
func (s *LogSource) RemoveInput(input string) {
	s.lock.Lock()
	delete(s.inputs, input)
	s.lock.Unlock()
}

// GetInputs returns the inputs handled by this source.
func (s *LogSource) GetInputs() []string {
	s.lock.Lock()
	defer s.lock.Unlock()
	inputs := make([]string, 0, len(s.inputs))
	for input := range s.inputs {
		inputs = append(inputs, input)
	}
	return inputs
}

// SetSourceType sets a format that give information on how the source lines should be parsed
func (s *LogSource) SetSourceType(sourceType SourceType) {
	s.lock.Lock()
	s.sourceType = sourceType
	s.lock.Unlock()
}

// GetSourceType returns the sourceType used by this source
func (s *LogSource) GetSourceType() SourceType {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.sourceType
}

// RegisterInfo registers some info to display on the status page
func (s *LogSource) RegisterInfo(i status.InfoProvider) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.info[i.InfoKey()] = i
}

// GetInfo gets an InfoProvider instance by the key
func (s *LogSource) GetInfo(key string) status.InfoProvider {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.info[key]
}

// GetInfoStatus returns a primitive representation of the info for the status page
func (s *LogSource) GetInfoStatus() map[string][]string {
	s.lock.Lock()
	defer s.lock.Unlock()
	info := make(map[string][]string)

	for _, v := range s.info {
		if len(v.Info()) == 0 {
			continue
		}
		info[v.InfoKey()] = v.Info()
	}
	return info
}

// HideFromStatus hides the source from the status output
func (s *LogSource) HideFromStatus() {
	s.lock.Lock()
	s.hiddenFromStatus = true
	s.lock.Unlock()
}

// IsHiddenFromStatus returns true if this source should be hidden from the status output
func (s *LogSource) IsHiddenFromStatus() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.hiddenFromStatus
}

// Dump provides a multi-line dump of the LogSource contents, for debugging purposes
func (s *LogSource) Dump() string {
	if s == nil {
		return "&LogSource(nil)"
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "&LogsSource @ %p = {\n", s)
	fmt.Fprintf(&b, "\tName: %#v,\n", s.Name)
	fmt.Fprintf(&b, "\tConfig: %s,\n", strings.ReplaceAll(s.Config.Dump(), "\n", "\n\t"))
	fmt.Fprintf(&b, "\tStatus: %s,\n", strings.ReplaceAll(s.Status.Dump(), "\n", "\n\t"))
	fmt.Fprintf(&b, "\tinputs: %#v,\n", s.inputs)
	fmt.Fprintf(&b, "\tMessages: %#v,\n", s.Messages.GetMessages())
	fmt.Fprintf(&b, "\tsourceType: %#v,\n", s.sourceType)
	fmt.Fprintf(&b, "\tinfo: %#v,\n", s.info)
	fmt.Fprintf(&b, "\tparentSource: %p,\n", s.ParentSource)
	fmt.Fprintf(&b, "\tLatencyStats: %#v,\n", s.LatencyStats)
	fmt.Fprintf(&b, "\tBytesRead: %d,\n", s.BytesRead.Load())
	fmt.Fprintf(&b, "\thiddenFromStatus: %t,\n", s.hiddenFromStatus)
	fmt.Fprintf(&b, "}")
	return b.String()
}
