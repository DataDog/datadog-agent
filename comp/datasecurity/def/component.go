// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurity provides the data security agent component.
//
// The component subscribes to the DEBUG remote-config product and logs the
// raw payloads it receives. It exists as a thin scaffold meant to be grown
// into a full data-security feature; right now its single responsibility is
// observing RC traffic so the rest of the wiring (RC client, fx graph,
// CODEOWNERS) can be validated end-to-end.
package datasecurity

// team: sensitive-data-scanner

// Component is the data security component interface.
//
// It currently exposes no methods because the component is purely
// side-effectful: it subscribes to RC on startup and logs received configs.
// Methods will be added as functionality grows.
type Component interface{}
