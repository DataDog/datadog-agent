// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package wire parses DogStatsD messages using explicit startup options.
// It owns grammar and worker-local interning; server composition owns configuration,
// discovery, enrichment, listeners, and routing.
//
// A Parser is not safe for concurrent use. Its Float64ListPool can be shared by
// workers. Successful multi-value metrics borrow Values from that pool. The caller
// must finish consuming those values before returning them, exactly once, to the
// same pool. Parsing errors return any borrowed storage inside the parser.
package wire
