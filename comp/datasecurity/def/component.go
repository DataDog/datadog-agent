// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurity provides the data security agent component.
//
// The component subscribes to the DEBUG remote-config product on startup. Each
// payload carries scanning rules and a per-integration scan query. For each
// payload the component:
//   - initializes an SDS scanner from the rules,
//   - looks up the related postgres config and runs the scan query against it,
//   - scans the column-oriented result, builds the SDS result protobuf payload
//     and forwards it to the sds-result intake, and
//   - destroys the scanner.
package datasecurity

// team: sensitive-data-scanner

// Component is the data security component interface.
//
// It currently exposes no methods because the component is purely
// side-effectful: it subscribes to RC on startup and logs received configs.
// Methods will be added as functionality grows.
type Component interface{}
