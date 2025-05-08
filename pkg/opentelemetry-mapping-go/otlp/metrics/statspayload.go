// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/golang/protobuf/proto"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// keyStatsPayload is the key for the stats payload in the attributes map.
// This is used as Metric name and Attribute key.
const keyStatsPayload = "dd.internal.stats.payload"

// StatsToMetrics converts a StatsPayload to a pdata.Metrics
func (t *Translator) StatsToMetrics(sp *pb.StatsPayload) (pmetric.Metrics, error) {
	bytes, err := proto.Marshal(sp)
	if err != nil {
		t.logger.Error("Failed to marshal stats payload", zap.Error(err))
		return pmetric.NewMetrics(), err
	}
	mmx := pmetric.NewMetrics()
	rmx := mmx.ResourceMetrics().AppendEmpty()
	smx := rmx.ScopeMetrics().AppendEmpty()
	mslice := smx.Metrics()
	mx := mslice.AppendEmpty()
	mx.SetName(keyStatsPayload)
	sum := mx.SetEmptySum()
	sum.SetIsMonotonic(false)
	dp := sum.DataPoints().AppendEmpty()
	byteSlice := dp.Attributes().PutEmptyBytes(keyStatsPayload)
	byteSlice.Append(bytes...)
	return mmx, nil
}
