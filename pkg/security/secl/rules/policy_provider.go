// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/hashicorp/go-multierror"
)

const DefaultPolicyName = "default.policy"

// PolicyProvider defines a rule provider
type PolicyProvider interface {
	LoadPolicies() ([]*Policy, *multierror.Error)
	SetOnNewPoliciesReadyCb(func())

	Start()
	Close() error
}
