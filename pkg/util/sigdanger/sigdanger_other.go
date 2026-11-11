// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !aix

// Package sigdanger handles SIGDANGER (AIX paging space low); on non-AIX
// platforms Init is a no-op.
package sigdanger

// Init is a no-op on non-AIX platforms.
func Init() {}
