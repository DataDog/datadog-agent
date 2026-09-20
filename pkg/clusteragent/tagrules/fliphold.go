// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"strings"
	"sync"
	"time"
)

// flipHold debounces tag-value flips: a value that changes again within the
// hold keeps the previously written value, bounding write and tagger churn.
// State is in-memory; a controller restart re-initializes from the values
// currently written on entities.
type flipHold struct {
	mu     sync.Mutex
	states map[string]flipState
}

type flipState struct {
	value string
	since time.Time
}

func newFlipHold() *flipHold {
	return &flipHold{states: map[string]flipState{}}
}

func flipKey(entityID, tagKey string) string {
	return entityID + "|" + tagKey
}

// resolve returns the value to write for an entity/tag key given the newly
// computed value ("" means the tag is dropped) and reports whether it differs
// from the previously written value. now is injectable for tests.
func (h *flipHold) resolve(entityID, tagKey, newValue string, hold time.Duration, now time.Time) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := flipKey(entityID, tagKey)
	state, known := h.states[key]
	if !known {
		h.states[key] = flipState{value: newValue, since: now}
		return newValue, true
	}
	if state.value == newValue {
		return newValue, false
	}
	if now.Sub(state.since) >= hold {
		h.states[key] = flipState{value: newValue, since: now}
		return newValue, true
	}
	return state.value, false
}

// drop releases flip state for a tag key, for example when its rule is
// deleted.
func (h *flipHold) drop(entityID, tagKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.states, flipKey(entityID, tagKey))
}

// dropTagKey releases flip state for every entity of a tag key, used when the
// owning rule is deleted.
func (h *flipHold) dropTagKey(tagKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key := range h.states {
		if _, entityTag, _ := strings.Cut(key, "|"); entityTag == tagKey {
			delete(h.states, key)
		}
	}
}
