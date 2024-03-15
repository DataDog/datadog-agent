// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

//nolint:revive // TODO(DBM) Fix revive linter
package common

// IntegrationName is the name of the integration.
const IntegrationName = "oracle"

/* We are temporarily using the name `oracle-dbm` to avoid scheduling clashes with the existing Oracle integration
 * functionality written in Python. We will change this back to `oracle` once we migrated this functionality
 * here.
 */
const IntegrationNameScheduler = "oracle-dbm"

//nolint:revive // TODO(DBM) Fix revive linter
const Godror = "godror"

//nolint:revive // TODO(DBM) Fix revive linter
const GoOra = "oracle"
