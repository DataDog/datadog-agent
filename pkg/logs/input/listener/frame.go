// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// TCP and UDP listeners are limited by the size of their read buffer, if the content of the message is bigger than the buffer length,
// it will arbitrary be truncated.
// For examples for |MSG| := |F1|F2|F3|:
//
// sending: |F1|
// sending: |F2|
// sending: |F3|
// would result in sending |MSG| to the logs-backend.
//
// sending: |MSG|
// would result in sending |F1| to the logs-backend.
//
// The size of the read buffer is configurable using the `frame_size` attribute for your `UDP` and `TCP` integrations.

// defaultMaxFrameSize represents the maximum size of a frame that can be sent over a network socket at a time, if the frame is bigger,
// the message will be truncated.
const defaultMaxFrameSize = 9000

// getMaxFrameSize returns the maximum frame size for a source.
func getMaxFrameSize(source *config.LogSource) int {
	if source.Config.FrameSize != 0 {
		return source.Config.FrameSize
	}
	return defaultMaxFrameSize
}

// getContent truncates the frame if it's too big.
func getContent(frame []byte, maxFrameSize int) []byte {
	content := make([]byte, len(frame))
	copy(content, frame)
	if len(frame) > maxFrameSize {
		content[maxFrameSize-1] = '\n'
		content = content[:maxFrameSize]
	}
	return content
}
