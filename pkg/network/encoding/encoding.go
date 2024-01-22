// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package encoding contains two subpackages marshal and unmarshal.
// They are separate because they are used in two different contexts and only share the common protobuf format,
// which comes from the github.com/DataDog/agent-payload package.
//
// The unmarshaller is used in the agent/process-agent to unmarshal data retrieved from system-probe.
// The marshaller is used only in system-probe to marshal its internal types to the common json/protobuf formats.
//
// If you combine the subpackages, then you end up having the agent/process-agent importing all the internal types
// from system-probe, which it has no need for. This results in a situation where additional imports in some
// seemingly unrelated system-probe packages, cause additional imports in agent/process-agent for no reason, which
// also increases the binary size.
package encoding
