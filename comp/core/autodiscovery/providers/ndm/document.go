// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// parseDocument splits an NDM document into its top-level feature keys,
// leaving each value untouched for its handler to unmarshal.
func parseDocument(raw []byte) (map[string]json.RawMessage, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("the payload is not a JSON object: %w", err)
	}
	if keys == nil {
		// Unmarshalling JSON null into a map succeeds and yields a nil map.
		return nil, errors.New("the payload is JSON null, not an object")
	}
	return keys, nil
}

// owns reports whether a document carries any registered key, which is the
// whole ownership test.
func (p *Provider) owns(keys map[string]json.RawMessage) bool {
	for _, key := range p.keys {
		if _, present := keys[key]; present {
			return true
		}
	}
	return false
}

// dispatch hands each rendered key present in the document to its handler.
// The caller must hold p.stateMutex for writing.
func (p *Provider) dispatch(path string, keys map[string]json.RawMessage) (map[string][]integration.Config, map[string]error) {
	var (
		configsByKey map[string][]integration.Config
		errsByKey    map[string]error
	)

	for _, key := range p.keys {
		renderer, rendered := p.renderers[key]
		if !rendered {
			continue
		}
		raw, present := keys[key]
		if !present {
			continue
		}

		configs, err := renderer.Render(path, raw)
		if err != nil {
			if errsByKey == nil {
				errsByKey = make(map[string]error, 1)
			}
			errsByKey[key] = err
		}
		if len(configs) > 0 {
			if configsByKey == nil {
				configsByKey = make(map[string][]integration.Config, 1)
			}
			configsByKey[key] = configs
		}
	}

	return configsByKey, errsByKey
}

// snapshot calls every snapshot handler once with the paths that carry its
// key, and returns what each path failed to apply, indexed by path then key.
func (p *Provider) snapshot(owned map[string]map[string]json.RawMessage) map[string]map[string]error {
	var errsByPath map[string]map[string]error

	for _, key := range p.keys {
		snapshotter, snapshotted := p.snapshotters[key]
		if !snapshotted {
			continue
		}

		docs := make(map[string]json.RawMessage)
		for path, keys := range owned {
			if raw, present := keys[key]; present {
				docs[path] = raw
			}
		}

		for path, err := range snapshotter.Snapshot(docs) {
			if err == nil {
				continue
			}
			if errsByPath == nil {
				errsByPath = make(map[string]map[string]error, 1)
			}
			if errsByPath[path] == nil {
				errsByPath[path] = make(map[string]error, 1)
			}
			errsByPath[path][key] = err
		}
	}

	return errsByPath
}

// applyStatus aggregates a path's per-key results into the one apply state
// Remote Configuration allows per path.
func applyStatus(errsByKey map[string]error, keys []string) state.ApplyStatus {
	if len(errsByKey) == 0 {
		return state.ApplyStatus{State: state.ApplyStateAcknowledged}
	}

	messages := make([]string, 0, len(errsByKey))
	for _, key := range keys {
		if err, failed := errsByKey[key]; failed {
			messages = append(messages, key+": "+err.Error())
		}
	}
	return state.ApplyStatus{
		State: state.ApplyStateError,
		Error: strings.Join(messages, "; "),
	}
}

// errorSet renders a path's per-key errors for GetConfigErrors.
func errorSet(errsByKey map[string]error, keys []string) types.ErrorMsgSet {
	if len(errsByKey) == 0 {
		return nil
	}

	set := make(types.ErrorMsgSet, len(errsByKey))
	for _, key := range keys {
		if err, failed := errsByKey[key]; failed {
			set[key+": "+err.Error()] = struct{}{}
		}
	}
	return set
}
