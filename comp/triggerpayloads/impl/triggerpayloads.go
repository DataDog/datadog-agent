// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package triggerpayloadsimpl implements the triggerpayloads component interface
package triggerpayloadsimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	egress "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	triggerpayloads "github.com/DataDog/datadog-agent/comp/triggerpayloads/def"
)

const (
	payloadsArg    = "payloads"
	triggerTimeout = time.Minute
)

// Requires defines the dependencies for the triggerpayloads component
type Requires struct {
	Log            log.Component
	InventoryAgent inventoryagent.Component
	HealthEgress   egress.Component
}

// Provides defines the output of the triggerpayloads component
type Provides struct {
	Comp       triggerpayloads.Component
	RCListener rcclienttypes.TaskListenerProvider
}

type sendFunc func(ctx context.Context) error

type triggerPayloads struct {
	log      log.Component
	payloads map[string]sendFunc
}

// NewComponent creates a new triggerpayloads component
func NewComponent(reqs Requires) Provides {
	t := &triggerPayloads{
		log: reqs.Log,
		payloads: map[string]sendFunc{
			triggerpayloads.PayloadInventoryMetadata: func(context.Context) error { return reqs.InventoryAgent.SendNow() },
			triggerpayloads.PayloadAgentHealth:       reqs.HealthEgress.SendNow,
		},
	}

	return Provides{
		Comp:       t,
		RCListener: rcclienttypes.NewTaskListener(t.handleAgentTask),
	}
}

func (t *triggerPayloads) handleAgentTask(taskType rcclienttypes.TaskType, task rcclienttypes.AgentTaskConfig) (bool, error) {
	if taskType != rcclienttypes.TaskTriggerPayloads {
		return false, nil
	}

	var payloads []string
	if raw, ok := task.Config.RawTaskArgs[payloadsArg]; ok {
		if err := json.Unmarshal(raw, &payloads); err != nil {
			return true, fmt.Errorf("invalid %q argument: %w", payloadsArg, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), triggerTimeout)
	defer cancel()
	return true, t.Trigger(ctx, payloads)
}

// Trigger sends the given payloads immediately and in parallel. An empty list sends all the payloads.
func (t *triggerPayloads) Trigger(ctx context.Context, payloads []string) error {
	if len(payloads) == 0 {
		for name := range t.payloads {
			payloads = append(payloads, name)
		}
	} else {
		payloads = slices.Clone(payloads)
	}
	slices.Sort(payloads)
	payloads = slices.Compact(payloads)

	errs := make([]error, len(payloads))
	var wg sync.WaitGroup
	for idx, name := range payloads {
		send, ok := t.payloads[name]
		if !ok {
			errs[idx] = fmt.Errorf("%s: unknown payload", name)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := send(ctx); err != nil {
				errs[idx] = fmt.Errorf("%s: %w", name, err)
			}
		}()
	}
	wg.Wait()

	var failed []error
	for idx, err := range errs {
		if err != nil {
			t.log.Warnf("Failed to trigger payload %s: %v", payloads[idx], err)
			failed = append(failed, err)
		}
	}

	switch len(failed) {
	case 0:
		t.log.Infof("Triggered payloads: %v", payloads)
		return nil
	case len(payloads):
		return errors.Join(failed...)
	default:
		return rcclienttypes.NewPartialFailureError(errors.Join(failed...))
	}
}
