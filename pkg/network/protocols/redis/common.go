// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

// TLSSetting is an alias for bool to represent whether the connection should be
// encrypted with TLS.
type TLSSetting = bool

// Constants to represent the different connection types.
const (
	Plaintext = false
	TLS       = true
)
