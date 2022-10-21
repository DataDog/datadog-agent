package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

type BaseHandler struct{}

func (BaseHandler) BeforeCacheCheck(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

func (BaseHandler) BeforeMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

func (BaseHandler) ScrubBeforeMarshalling(ctx *processors.ProcessorContext, resource interface{}) {
}
