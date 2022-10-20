package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"k8s.io/apimachinery/pkg/types"
)

type BaseHandler struct{}

func (BaseHandler) AfterMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) BeforeCacheCheck(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

func (BaseHandler) BeforeMarshalling(ctx *processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	return
}

func (BaseHandler) BuildMessageBody(ctx *processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ExtractResource(ctx *processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ResourceList(ctx *processors.ProcessorContext, list interface{}) (resources []interface{}) {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ResourceUID(ctx *processors.ProcessorContext, resource, resourceModel interface{}) types.UID {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ResourceVersion(ctx *processors.ProcessorContext, resource, resourceModel interface{}) string {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ScrubBeforeExtraction(ctx *processors.ProcessorContext, resource interface{}) {
	//TODO implement me
	panic("implement me")
}

func (BaseHandler) ScrubBeforeMarshalling(ctx *processors.ProcessorContext, resource interface{}) {
}
