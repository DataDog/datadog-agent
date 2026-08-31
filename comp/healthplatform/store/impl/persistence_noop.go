// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package storeimpl

import "context"

// noopPersistence is used when neither durable disk nor remote restoration is
// appropriate for the current process. It silently discards all writes and
// returns no state on load.
type noopPersistence struct{}

func (n *noopPersistence) load(_ context.Context) (*PersistedState, error) { return nil, nil }
func (n *noopPersistence) save(_ *PersistedState) error                    { return nil }
