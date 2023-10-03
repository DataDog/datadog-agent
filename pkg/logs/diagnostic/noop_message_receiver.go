// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic/module"
)

// NoopMessageReceiver for cases where diagnosing messages is unsupported or not needed (serverless, tests)
type NoopMessageReceiver = module.NoopMessageReceiver

// HandleMessage does nothing with the message
var HandleMessage = (*NoopMessageReceiver).HandleMessage
