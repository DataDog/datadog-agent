// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package restart

// Startable represents a startable object
type Startable interface {
	Start()
}

// Starter starts a group of startable objects from a data pipeline
type Starter interface {
	Startable
	Add(components ...Startable)
}
