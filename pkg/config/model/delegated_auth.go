// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

// DelaDirectivePrefix is the prefix that marks a value in an additional_endpoints API key
// slot as a delegated-auth directive rather than a literal key. It is defined here — in the
// lowest-level config package — so that pkg/config/utils, pkg/config/setup, and
// comp/logs/agent/config can all reference the same constant instead of each hard-coding
// "DELA(" and risking silent drift.
const DelaDirectivePrefix = "DELA("
