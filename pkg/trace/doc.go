// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace encapsulates a module which contains the entirety of the trace-agent's processing pipeline. The code
// may be reused to process traces in the same way that the Datadog Agent does, but outside of it.
//
// Please note that the API is subject to major changes and should not be relied upon as being stable.
package trace
