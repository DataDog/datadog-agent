package topn

import "github.com/DataDog/datadog-agent/comp/netflow/common"

type NoopFilter struct {
}

func (n NoopFilter) Filter(ctx common.FlushContext, flows []*common.Flow) []*common.Flow {
	return flows
}
