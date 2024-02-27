// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle_dbm

// CheckName is the name of the check that was renamed to `oracle`.
// This is used to keep the compatibility with the old configuration.
const CheckName = "oracle-dbm"
