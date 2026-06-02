// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurity provides the data security agent component.
//
// The component subscribes to the DEBUG remote-config product. For payloads
// whose product_type is "data_security" it takes over the matching postgres
// config (a postgres instance targeting the payload host that opted in with
// data_security.enabled: true): the original (file-provided) config is
// unscheduled and an enriched copy, with the rules merged into the instance's
// data_security section, is scheduled in its place so a single enriched
// postgres check runs. When the RC config goes away the original config is
// restored. Any other product_type is logged and ignored.
package datasecurity

// team: sensitive-data-scanner

// Component is the data security component interface.
//
// It currently exposes no methods because the component is purely
// side-effectful: it subscribes to RC on startup and logs received configs.
// Methods will be added as functionality grows.
type Component interface{}
