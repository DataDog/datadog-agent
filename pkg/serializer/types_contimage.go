// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package serializer

import (
	"github.com/DataDog/agent-payload/v5/contimage"
)

// ContainerImageMessage is a type alias for contimage proto payload
type ContainerImageMessage = contimage.ContainerImagePayload
