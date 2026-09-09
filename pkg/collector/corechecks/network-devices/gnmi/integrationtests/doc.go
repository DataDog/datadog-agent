// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

/*
Package integrationtests contains local integration tests for the gNMI core check.

These tests exercise the check through the same loading path the Agent uses
(file-based conf.d configuration, GoCheckLoader, and the aggregator sender
demultiplexer) while talking to the in-process fakeserver on loopback.

Limitation: these tests do not spawn the Agent binary or run the collector
scheduler. Full process-level coverage lives in new-e2e when a GNMI scenario
is added. This package is the strongest self-contained local integration
pattern used by other core checks (for example SNMP profile metadata tests).
*/
package integrationtests
