// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/collector/check/types"
)

type Check = types.Check
type Info = types.Info

// ErrSkipCheckInstance is returned from Configure() when a check is intentionally refusing to load a
// check instance, and NOT due to an error. The distinction is important for deciding whether or not
// to log the error and report it on the status page.
//
// Loaders should check for this error after calling Configure() and should not log it as an error.
// The scheduler reports this error to the agent status if and only if all loaders fail to load the
// check instance.
//
// Usage example: one version of the check is written in Python, and another is written in Golang. Each
// loader is called on the given configuration, and will reject the configuration if it does not
// match the right version, without raising errors to the log or agent status. If another error is
// returned then the errors will be properly logged and reported in the agent status.
var ErrSkipCheckInstance = errors.New("refused to load the check instance")
