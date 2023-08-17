// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tailerfactory

// Tailer abstracts types which can tail logs.
type Tailer interface {

	// Start starts the tailer.  If this returns an error, the tailer is not started.
	Start() error

	// Stop stops the tailer, waiting for the operations to finish.
	Stop()
}
