// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

import "fmt"

// ddotVersion records, for a given Datadog Agent release, the OpenTelemetry
// Collector versions that the DDOT (Datadog Distribution of OpenTelemetry)
// collector ships with.
//
// The OCB manifest pins components to two distinct version streams:
//   - the stable "v1.x" stream (OTel core modules that reached 1.0), and
//   - the "v0.x" stream (OTel core beta modules and all contrib modules).
//
// CoreStableVersion is applied to components currently on the v1.x stream and
// ContribVersion to everything else. OCBVersion is written to dist.version.
type ddotVersion struct {
	AgentVersion      string // e.g. "7.83.2"
	OCBVersion        string // e.g. "0.158.0"  -> dist.version
	CoreStableVersion string // e.g. "v1.64.0"  -> components currently on v1.x
	ContribVersion    string // e.g. "v0.158.0" -> components currently on v0.x
}

// ddotVersions is the static compatibility table. It intentionally avoids any
// network lookup: each Agent release maps to a fixed, reproducible set of
// collector versions. Add a new row for every DDOT release.
//
// Seeded from git history of comp/otelcol/collector-contrib/impl/manifest.yaml.
// The CoreStableVersion follows the invariant: minor(CoreStable) = minor(OCB) - 94.
var ddotVersions = map[string]ddotVersion{
	"7.84.0": {OCBVersion: "0.159.0", CoreStableVersion: "v1.65.0", ContribVersion: "v0.159.0"},
	"7.83.3": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", ContribVersion: "v0.158.0"},
	"7.83.2": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", ContribVersion: "v0.158.0"},
	"7.83.1": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", ContribVersion: "v0.158.0"},
	"7.83.0": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", ContribVersion: "v0.158.0"},
	"7.82.3": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", ContribVersion: "v0.155.0"},
	"7.82.2": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", ContribVersion: "v0.155.0"},
	"7.82.1": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", ContribVersion: "v0.155.0"},
	"7.82.0": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", ContribVersion: "v0.155.0"},
	"7.81.3": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", ContribVersion: "v0.154.0"},
	"7.81.2": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", ContribVersion: "v0.154.0"},
	"7.81.1": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", ContribVersion: "v0.154.0"},
	"7.81.0": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", ContribVersion: "v0.154.0"},
	"7.80.4": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", ContribVersion: "v0.152.0"},
	"7.80.3": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", ContribVersion: "v0.152.0"},
	"7.80.2": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", ContribVersion: "v0.152.0"},
	"7.80.1": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", ContribVersion: "v0.152.0"},
	"7.80.0": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", ContribVersion: "v0.152.0"},
	"7.79.2": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", ContribVersion: "v0.150.0"},
	"7.79.1": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", ContribVersion: "v0.150.0"},
	"7.79.0": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", ContribVersion: "v0.150.0"},
	"7.78.4": {OCBVersion: "0.147.0", CoreStableVersion: "v1.53.0", ContribVersion: "v0.147.0"},
	"7.78.3": {OCBVersion: "0.147.0", CoreStableVersion: "v1.53.0", ContribVersion: "v0.147.0"},
}

// lookupVersion returns the compatibility row for the given Agent version, or
// an error listing the known versions when it is not in the table.
func lookupVersion(agentVersion string) (ddotVersion, error) {
	v, ok := ddotVersions[agentVersion]
	if ok {
		v.AgentVersion = agentVersion
		return v, nil
	}
	known := make([]string, 0, len(ddotVersions))
	for key := range ddotVersions {
		known = append(known, key)
	}
	return ddotVersion{}, fmt.Errorf("unknown agent version %q; known versions: %v", agentVersion, known)
}
