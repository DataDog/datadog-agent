// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaler jsonpb.Marshaler
}

func (j jsonSerializer) Marshal(stats map[int32]*procutil.StatsWithPerm) ([]byte, error) {
	writer := new(bytes.Buffer)
	payload := &model.ProcStatsWithPermByPID{
		StatsByPID: make(map[int32]*model.ProcStatsWithPerm),
	}

	for pid, s := range stats {
		stat := statPool.Get()
		stat.OpenFDCount = s.OpenFdCount
		stat.ReadCount = s.IOStat.ReadCount
		stat.WriteCount = s.IOStat.WriteCount
		stat.ReadBytes = s.IOStat.ReadBytes
		stat.WriteBytes = s.IOStat.WriteBytes
		payload.StatsByPID[pid] = stat
	}

	err := j.marshaler.Marshal(writer, payload)
	returnToPool(payload.StatsByPID)
	return writer.Bytes(), err
}

func (jsonSerializer) Unmarshal(blob []byte) (*model.ProcStatsWithPermByPID, error) {
	stats := new(model.ProcStatsWithPermByPID)
	reader := bytes.NewReader(blob)
	if err := jsonpb.Unmarshal(reader, stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
var _ Unmarshaler = jsonSerializer{}
