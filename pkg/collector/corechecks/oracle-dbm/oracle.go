// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package oracledbm contains the oracle check. The oracle check was renamed from oracle-dbm
// to oracle. This package is used to keep the compatibility with the old configuration. It essentially
// just executes the oracle check.
package oracledbm

// CheckName is the name of the check that was renamed to `oracle`.
const CheckName = "oracle-dbm"
