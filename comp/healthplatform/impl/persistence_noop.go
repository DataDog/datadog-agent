// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

// noopPersistence is used in environments where disk persistence is not meaningful,
// such as Kubernetes pods backed by emptyDir volumes where state is lost on restart anyway.
// It silently discards all writes and returns no state on load.
type noopPersistence struct{}

func (n *noopPersistence) load() (*PersistedState, error) { return nil, nil }
func (n *noopPersistence) save(_ *PersistedState) error   { return nil }
