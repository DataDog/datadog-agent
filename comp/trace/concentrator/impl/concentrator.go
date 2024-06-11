package impl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/trace/concentrator/def"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// NewConcentrator initializes a new concentrator ready to be started
func NewConcentrator(conf config.Component, out chan *pb.StatsPayload, now time.Time, statsd statsd.ClientInterface) def.Concentrator {
	return stats.NewConcentrator(conf.Object(), out, time.Now(), statsd)
}
