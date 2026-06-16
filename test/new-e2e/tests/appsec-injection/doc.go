// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package appsecinjection provides end-to-end tests for the Datadog AppSec
// Injection Proxy feature.  The suite in this package covers Envoy Gateway
// sidecar mode over a Unix Domain Socket (UDS), including traffic-blocking
// assertions and a reconcile-loop stability guard.
package appsecinjection
