// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ccmmode contains e2e tests for cloud_cost_only infrastructure mode tagging.
// Tests verify that the infra_mode:cloud_cost_only tag is applied to integration
// metrics from all checks (default) or from a selective list (configured tagged: [...]).
package ccmmode
