// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package signingkeysimpl implements the Core Agent's PAR signing-key snapshot.
package signingkeysimpl

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	signingkeysdef "github.com/DataDog/datadog-agent/comp/privateactionrunner/signingkeys/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/signingkeys"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var errUnavailable = errors.New("PAR signing-key snapshot is unavailable")

type component struct {
	mu           sync.RWMutex
	snapshot     signingkeysdef.Snapshot
	internalErr  error
	wasConnected bool
}

// Provides contains the component and its Remote Config listener.
type Provides struct {
	compdef.Out

	Comp     signingkeysdef.Component
	Listener rcclienttypes.RCStatusListener `group:"rCStatusListener"`
}

// NewComponent creates the signing-key snapshot component.
func NewComponent() Provides {
	c := &component{}
	return Provides{
		Comp: c,
		Listener: rcclienttypes.RCStatusListener{
			Product:       data.Product(state.ProductActionPlatformRunnerKeys),
			OnStatus:      c.onStatus,
			OnStateChange: c.onStateChange,
		},
	}
}

func (c *component) Get(knownRevision uint64) (signingkeysdef.Snapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.internalErr != nil {
		return signingkeysdef.Snapshot{}, fmt.Errorf("%w: %v", errUnavailable, c.internalErr)
	}

	snapshot := c.snapshot
	snapshot.Keys = signingkeys.Clone(snapshot.Keys)
	snapshot.Unchanged = snapshot.Initialized && knownRevision == snapshot.Revision
	return snapshot, nil
}

func (c *component) onStatus(configs map[string]state.RawConfig, status pbgo.ConfigStatus, apply func(string, state.ApplyStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wasConnected = true

	if status == pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED {
		c.internalErr = nil
		if c.snapshot.ConfigStatus != status || c.snapshot.Revision == 0 {
			c.snapshot.ConfigStatus = status
			c.snapshot.Revision++
			c.snapshot.UpdatedAt = time.Now()
		}
		return
	}

	keys, decodeErrs := signingkeys.DecodeSnapshot(configs)
	if len(decodeErrs) != 0 {
		for configID := range configs {
			err := decodeErrs[configID]
			if err == nil {
				err = errors.New("complete signing-key snapshot rejected")
			}
			apply(configID, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		}
		c.internalErr = errors.New("invalid AP_RUNNER_KEYS snapshot")
		return
	}
	for configID := range configs {
		apply(configID, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	changed := !c.snapshot.Initialized || c.snapshot.ConfigStatus != status || !reflect.DeepEqual(c.snapshot.Keys, keys)
	c.internalErr = nil
	if !changed {
		return
	}
	c.snapshot.Keys = signingkeys.Clone(keys)
	c.snapshot.ConfigStatus = status
	c.snapshot.Initialized = true
	c.snapshot.Revision++
	c.snapshot.UpdatedAt = time.Now()
}

func (c *component) onStateChange(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if connected {
		c.wasConnected = true
		return
	}
	if c.wasConnected {
		c.internalErr = errors.New("Remote Config connection lost")
	}
}
