// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretsimpl

import (
	"errors"
	"slices"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
)

// Keep the existing local error while recording only a fixed reason for Agent Health.
type lookupError struct {
	error
	reason string
}

// Called under the resolver lock, at the backend lookup boundary.
func (r *secretResolver) recordResolutionFailure(handle string, err error) {
	reason := "backend_error"
	var failure *lookupError
	if errors.As(err, &failure) {
		reason = failure.reason
	}
	r.resolutionFailures[handle] = reason
}

// SetOriginName associates a configuration digest with its integration name.
func (r *secretResolver) SetOriginName(origin, name string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.originNames[origin] = name
}

// GetResolutionFailures returns a snapshot of active failed lookups.
func (r *secretResolver) GetResolutionFailures() []secrets.ResolutionFailure {
	r.lock.Lock()
	defer r.lock.Unlock()
	var failures []secrets.ResolutionFailure
	for handle, reason := range r.resolutionFailures {
		_, cached := r.cache[handle]
		for _, ref := range r.origin[handle] {
			name := r.originNames[ref.origin]
			if name == "" {
				name = ref.origin
			}
			failures = append(failures, secrets.ResolutionFailure{
				Handle: handle, Origin: ref.origin, OriginName: name,
				Path: slices.Clone(ref.path), Reason: reason, HasCachedValue: cached,
			})
		}
	}
	return failures
}
