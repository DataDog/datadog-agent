// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContainerImage(t *testing.T) {
	tests := []struct {
		name                      string
		imageName                 string
		expectedWorkloadmetaImage ContainerImage
		expectsErr                bool
	}{
		{
			name:      "image with tag",
			imageName: "datadog/agent:7",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
				ID:        "0",
			},
		}, {
			name:      "image without tag",
			imageName: "datadog/agent",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "latest", // Default to latest when there's no tag
				ID:        "1",
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			image, err := NewContainerImage(strconv.Itoa(i), test.imageName)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedWorkloadmetaImage, image)
		})
	}
}
