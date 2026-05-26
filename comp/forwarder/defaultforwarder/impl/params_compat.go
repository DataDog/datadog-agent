// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarderimpl

import defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"

// Params is an alias for defaultforwarderdef.Params for backward compatibility within this package.
type Params = defaultforwarderdef.Params

// NewParams creates a new Params struct.
var NewParams = defaultforwarderdef.NewParams

// WithResolvers enables the forwarder to use resolvers.
var WithResolvers = defaultforwarderdef.WithResolvers

// WithDisableAPIKeyChecking disables the API key checking.
var WithDisableAPIKeyChecking = defaultforwarderdef.WithDisableAPIKeyChecking

// WithFeatures sets features on the forwarder.
var WithFeatures = defaultforwarderdef.WithFeatures
