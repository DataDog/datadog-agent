// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package locallogtailer provides a self-contained log tailing pipeline for the
// observer. It discovers log sources via AutoDiscovery, tails files locally,
// and forwards processed messages to the observer without shipping to the
// Datadog backend.
package locallogtailer

// team: q-branch

// Component is the component type.
type Component interface{}
