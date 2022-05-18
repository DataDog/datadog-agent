// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

// PolicyProvider defines a rule provider
type PolicyProvider interface {
	LoadPolicy() (*Policy, error)
	SetOnPolicyChangedCb(_ func(*Policy))
	//GetPriority() int
}
