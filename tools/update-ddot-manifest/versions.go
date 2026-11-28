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
// Both the core (go.opentelemetry.io/collector) and contrib
// (opentelemetry-collector-contrib) repositories publish two version streams:
//   - a stable "v1.x" stream for modules that reached 1.0, and
//   - an unstable "v0.x" stream for everything still in beta.
//
// A component therefore falls into one of four cells for a given release —
// {core,contrib} × {stable,unstable}. Which cell applies to a component is
// decided per component in components.go (its stream and first stable version);
// this struct only holds the four version numbers of the release. OCBVersion is
// written to dist.version.
//
// The four numbers may coincide (e.g. core and contrib unstable are both
// v0.159.0), which is exactly why the stream must be stored per component rather
// than inferred from a version string.
type ddotVersion struct {
	AgentVersion           string // e.g. "7.83.2"
	OCBVersion             string // e.g. "0.158.0"  -> dist.version
	CoreStableVersion      string // e.g. "v1.64.0"  -> core module on its v1.x stream
	CoreUnstableVersion    string // e.g. "v0.158.0" -> core module still in beta
	ContribStableVersion   string // e.g. "v1.64.0"  -> contrib module on its v1.x stream
	ContribUnstableVersion string // e.g. "v0.158.0" -> contrib module still in beta
}

// ddotVersions is the static compatibility table. It intentionally avoids any
// network lookup: each Agent release maps to a fixed, reproducible set of
// collector versions. Add a new row for every DDOT release.
//
// Seeded from git history of comp/otelcol/collector-contrib/impl/manifest.yaml.
// When adding a release, take the real OCB / core-stable / unstable numbers from
// the upstream release (do not compute them from one another: the core stable and
// beta trains are released in lockstep but their minor offset is not a guaranteed
// invariant).
//
// ContribStableVersion is empty for every row below: the contrib repository's
// stable (v1.x) stream only began after the most recent release listed here
// (contrib v1.0.0 was tagged after v0.159.0), so no contrib module resolves to a
// stable version at these releases. The contrib stable stream has its own lockstep
// counter, independent of core's (e.g. contrib v1.x vs core v1.65.x); when a
// future DDOT release ships a stable contrib module, set ContribStableVersion to
// that release's contrib v1.x train version.
var ddotVersions = map[string]ddotVersion{
	"7.84.0": {OCBVersion: "0.159.0", CoreStableVersion: "v1.65.0", CoreUnstableVersion: "v0.159.0", ContribStableVersion: "", ContribUnstableVersion: "v0.159.0"},
	"7.83.3": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", CoreUnstableVersion: "v0.158.0", ContribStableVersion: "", ContribUnstableVersion: "v0.158.0"},
	"7.83.2": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", CoreUnstableVersion: "v0.158.0", ContribStableVersion: "", ContribUnstableVersion: "v0.158.0"},
	"7.83.1": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", CoreUnstableVersion: "v0.158.0", ContribStableVersion: "", ContribUnstableVersion: "v0.158.0"},
	"7.83.0": {OCBVersion: "0.158.0", CoreStableVersion: "v1.64.0", CoreUnstableVersion: "v0.158.0", ContribStableVersion: "", ContribUnstableVersion: "v0.158.0"},
	"7.82.3": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", CoreUnstableVersion: "v0.155.0", ContribStableVersion: "", ContribUnstableVersion: "v0.155.0"},
	"7.82.2": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", CoreUnstableVersion: "v0.155.0", ContribStableVersion: "", ContribUnstableVersion: "v0.155.0"},
	"7.82.1": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", CoreUnstableVersion: "v0.155.0", ContribStableVersion: "", ContribUnstableVersion: "v0.155.0"},
	"7.82.0": {OCBVersion: "0.155.0", CoreStableVersion: "v1.61.0", CoreUnstableVersion: "v0.155.0", ContribStableVersion: "", ContribUnstableVersion: "v0.155.0"},
	"7.81.3": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", CoreUnstableVersion: "v0.154.0", ContribStableVersion: "", ContribUnstableVersion: "v0.154.0"},
	"7.81.2": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", CoreUnstableVersion: "v0.154.0", ContribStableVersion: "", ContribUnstableVersion: "v0.154.0"},
	"7.81.1": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", CoreUnstableVersion: "v0.154.0", ContribStableVersion: "", ContribUnstableVersion: "v0.154.0"},
	"7.81.0": {OCBVersion: "0.154.0", CoreStableVersion: "v1.60.0", CoreUnstableVersion: "v0.154.0", ContribStableVersion: "", ContribUnstableVersion: "v0.154.0"},
	"7.80.4": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", CoreUnstableVersion: "v0.152.0", ContribStableVersion: "", ContribUnstableVersion: "v0.152.0"},
	"7.80.3": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", CoreUnstableVersion: "v0.152.0", ContribStableVersion: "", ContribUnstableVersion: "v0.152.0"},
	"7.80.2": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", CoreUnstableVersion: "v0.152.0", ContribStableVersion: "", ContribUnstableVersion: "v0.152.0"},
	"7.80.1": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", CoreUnstableVersion: "v0.152.0", ContribStableVersion: "", ContribUnstableVersion: "v0.152.0"},
	"7.80.0": {OCBVersion: "0.152.0", CoreStableVersion: "v1.58.0", CoreUnstableVersion: "v0.152.0", ContribStableVersion: "", ContribUnstableVersion: "v0.152.0"},
	"7.79.2": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", CoreUnstableVersion: "v0.150.0", ContribStableVersion: "", ContribUnstableVersion: "v0.150.0"},
	"7.79.1": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", CoreUnstableVersion: "v0.150.0", ContribStableVersion: "", ContribUnstableVersion: "v0.150.0"},
	"7.79.0": {OCBVersion: "0.150.0", CoreStableVersion: "v1.56.0", CoreUnstableVersion: "v0.150.0", ContribStableVersion: "", ContribUnstableVersion: "v0.150.0"},
	"7.78.4": {OCBVersion: "0.147.0", CoreStableVersion: "v1.53.0", CoreUnstableVersion: "v0.147.0", ContribStableVersion: "", ContribUnstableVersion: "v0.147.0"},
	"7.78.3": {OCBVersion: "0.147.0", CoreStableVersion: "v1.53.0", CoreUnstableVersion: "v0.147.0", ContribStableVersion: "", ContribUnstableVersion: "v0.147.0"},
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
