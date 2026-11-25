// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// configKeyHandler is a FlagHandler that drives one boolean configuration key.
type configKeyHandler struct {
	name    FlagName
	setter  ConfigSetter
	key     string
	healthy func() bool

	// mu serializes the read-then-write of the key, because the client may run
	// OnChange, OnNoConfig and SafeRecover from different goroutines.
	mu sync.Mutex
}

// NewConfigKeyHandler returns a FlagHandler that drives the boolean
// configuration key through setter, using the Remote Config source.
//
// When the flag becomes true, the key is set to true only if no other source
// already provides a value for it. A value from a file, an environment
// variable, a fleet policy or the agent runtime is left untouched, so a local
// explicit false keeps the feature off and a local true stays on.
//
// When the flag becomes false, is removed, or the client asks the handler to
// recover, the Remote Config value for the key is removed. The handler never
// writes false: the key falls back to whatever the other sources provide, and a
// local true is never turned off.
//
// A local false that is set after the flag turned the key on does not take
// effect while the flag stays true, because the Remote Config layer has a
// higher priority than the local ones. It applies once the flag is turned off.
//
// The handler assumes it is the only writer of the Remote Config value for the
// key: releasing removes whatever value that source holds for the key.
//
// After a recovery the key stays off until the flag value changes again: the
// client does not notify a handler twice with the same value.
//
// healthy is reported by IsHealthy. A nil healthy function means the handler is
// always healthy.
func NewConfigKeyHandler(name FlagName, setter ConfigSetter, key string, healthy func() bool) FlagHandler {
	return &configKeyHandler{name: name, setter: setter, key: key, healthy: healthy}
}

// FlagName returns the name of the flag the handler listens to.
func (h *configKeyHandler) FlagName() FlagName { return h.name }

// OnChange turns the key on when value is true and no other source provides a
// value for it, and removes the Remote Config value when value is false.
func (h *configKeyHandler) OnChange(value FlagValue) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !value {
		h.release()
		return nil
	}
	if shouldApplyConfig(h.setter.GetSource(h.key), false) {
		h.setter.Set(h.key, true, model.SourceRC)
	}
	return nil
}

// OnNoConfig removes the Remote Config value for the key.
func (h *configKeyHandler) OnNoConfig() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.release()
}

// SafeRecover removes the Remote Config value for the key.
func (h *configKeyHandler) SafeRecover(error, FlagValue) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.release()
}

// IsHealthy reports the result of the health function given at construction,
// or true when there is none.
func (h *configKeyHandler) IsHealthy() bool {
	if h.healthy == nil {
		return true
	}
	return h.healthy()
}

// release gives the key back to the other sources. Must be called with mu held.
func (h *configKeyHandler) release() {
	h.setter.UnsetForSource(h.key, model.SourceRC)
}

// handlerList is a RemoteFlagSubscriber backed by a fixed slice of handlers.
type handlerList []FlagHandler

// Handlers returns the handlers of the list.
func (l handlerList) Handlers() []FlagHandler { return l }

// Subscribers returns a RemoteFlagSubscriber whose Handlers method returns
// handlers.
func Subscribers(handlers ...FlagHandler) RemoteFlagSubscriber {
	return handlerList(handlers)
}
