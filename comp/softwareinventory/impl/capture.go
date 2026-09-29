// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package softwareinventoryimpl

// CaptureTransformer is installed only for an explicit native capture. It must
// return an independent sanitized snapshot and must not mutate cached inventory.
type CaptureTransformer func(*Payload) (*Payload, error)
