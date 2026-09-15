// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

// Standard BRIDGE-MIB / Q-BRIDGE-MIB OIDs used for host-to-device FDB collection.
const (
	oidDot1dBasePortIfIndex = "1.3.6.1.2.1.17.1.4.1.2"
	oidDot1qTpFdbPort       = "1.3.6.1.2.1.17.7.1.2.2.1.2"
	oidDot1qTpFdbStatus     = "1.3.6.1.2.1.17.7.1.2.2.1.3"
	oidDot1dTpFdbPort       = "1.3.6.1.2.1.17.4.3.1.2"
	oidDot1dTpFdbStatus     = "1.3.6.1.2.1.17.4.3.1.3"

	fdbStatusLearned = 3
)
