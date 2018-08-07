// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"time"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/seek"
)

// Since returns the date from when logs should be collected.
func Since(seeker *seek.Seeker, container types.Container, identifier string) (time.Time, error) {
	var since time.Time
	var err error
	strategy, offset := seeker.Seek(time.Unix(container.Created, 0), identifier)
	switch strategy {
	case seek.Start:
		since = time.Time{}
	case seek.Recover:
		since, err = time.Parse(config.DateFormat, offset)
		if err != nil {
			return time.Now().UTC(), err
		}
		since = since.Add(time.Nanosecond)
	case seek.End:
		since = time.Now().UTC()
	}
	return since, nil
}
