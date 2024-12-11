// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package encoding

import (
	"strings"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

var (
	pSerializer = protoSerializer{}
	jSerializer = jsonSerializer{
		marshaler: jsonpb.Marshaler{
			EmitDefaults: true,
		},
	}
)

// Marshaler is an interface implemented by all stats serializers
type Marshaler interface {
	Marshal(stats map[int32]*procutil.StatsWithPerm) ([]byte, error)
	ContentType() string
}

// Unmarshaler is an interface implemented by all process stats deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*model.ProcStatsWithPermByPID, error)
}

// GetMarshaler returns the appropriate Marshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

var statPool = ddsync.NewDefaultTypedPool[model.ProcStatsWithPerm]()

func returnToPool(stats map[int32]*model.ProcStatsWithPerm) {
	for _, s := range stats {
		statPool.Put(s)
	}
}
