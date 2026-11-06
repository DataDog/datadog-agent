// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ottlfuncs contains the Agent's supported subset of upstream OTTL
// functions. Importing upstream ottlfuncs also initializes unused XML, grok, and
// user-agent dependencies. Keeping only the needed factories here avoids those
// side effects without replacing the upstream OTTL parser or changing semantics.
// See README.md for provenance and the update procedure.
package ottlfuncs
