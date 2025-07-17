package service

import "github.com/DataDog/datadog-agent/pkg/util/startstop"

var (
	_ startstop.StartStoppable = &noOpRunnable{}
)

// noOpRunnable is a no-op implementation of startstop.StartStoppable.
type noOpRunnable struct{}

func (s noOpRunnable) Start() {}
func (s noOpRunnable) Stop()  {}
