// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package inframetadata handles host metadata and infrastructure list related features. It stores the host metadata and gohai payload definitions as well as the `Reporter` implementation.
//
// A `Reporter` keeps a `HostMap` (a map of hostnames to host metadata payloads) and periodically clears it out and reports the information using a `Pusher`
//
// The `Reporter` has three public methods:
// - The `Run() error` and `Stop()` methods manage its lifecycle
// - The `ConsumeResource(pcommon.Resource) (bool, error)` method ingests resources, updates host metadata payloads, and reports whether any changes or errors occurred during processing.
//
// Internally, the `Reporter` manages a `HostMap`, which has two public methods:
// - The `Update(host string, resource pcommon.Resource) (changed bool, err error)` method updates a hosts information and reports whether any changes or errors occurred during processing.
// - The `Extract() map[string]payloads.HostMetadata` method clears out the `HostMap` and returns a copy of its internal information.
package inframetadata
