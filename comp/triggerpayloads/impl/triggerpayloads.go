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

	type result struct {
		idx int
		err error
	}
	// Buffered so senders still running after a timeout never block
	results := make(chan result, len(payloads))
	errs := make([]error, len(payloads))
	pending := 0
	for idx, name := range payloads {
		send, ok := t.payloads[name]
		if !ok {
			errs[idx] = fmt.Errorf("%s: unknown payload", name)
			continue
		}
		pending++
		go func() {
			results <- result{idx: idx, err: send(ctx)}
		}()
	}

	done := make([]bool, len(payloads))
	record := func(r result) {
		done[r.idx] = true
		if r.err != nil {
			errs[r.idx] = fmt.Errorf("%s: %w", payloads[r.idx], r.err)
		}
	}
wait:
	for ; pending > 0; pending-- {
		select {
		case r := <-results:
			record(r)
		case <-ctx.Done():
			// Keep the results that completed before the deadline
		drain:
			for {
				select {
				case r := <-results:
					record(r)
				default:
					break drain
				}
			}
			for idx, name := range payloads {
				if errs[idx] == nil && !done[idx] {
					errs[idx] = fmt.Errorf("%s: %w", name, ctx.Err())
				}
			}
			break wait
		}
	}

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
