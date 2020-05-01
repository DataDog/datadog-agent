package interpreter

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/pkg/errors"
	"strconv"
	"time"
)

const createTimeField = "span.starttime"
const hostnameField = "span.hostname"
const pidField = "span.pid"
const kindField = "span.kind"

// SpanInterpreterEngineContext helper functions that is used by the span interpreter engine context
type SpanInterpreterEngineContext interface {
	nanosToMillis(nanos int64) int64
	extractSpanMetadata(span *pb.Span) (*model.SpanMetadata, error)
}

type spanInterpreterEngineContext struct {
	Config *config.Config
}

// MakeSpanInterpreterEngineContext creates a SpanInterpreterEngineContext for config
func MakeSpanInterpreterEngineContext(config *config.Config) SpanInterpreterEngineContext {
	return &spanInterpreterEngineContext{Config: config}
}

func (c *spanInterpreterEngineContext) nanosToMillis(nanos int64) int64 {
	return nanos / int64(time.Millisecond)
}

func (c *spanInterpreterEngineContext) extractSpanMetadata(span *pb.Span) (*model.SpanMetadata, error) {

	var hostname string
	var createTime int64
	var pid int
	var kind string
	var found bool

	if hostname, found = span.Meta[hostnameField]; !found {
		return nil, createSpanMetadataError(hostnameField)
	}

	if pidStr, found := span.Meta[pidField]; found {
		p, err := strconv.Atoi(pidStr)
		if err != nil {
			return nil, err
		}
		pid = p
	} else {
		return nil, createSpanMetadataError(pidField)
	}

	if kind, found = span.Meta[kindField]; !found {
		return nil, createSpanMetadataError(kindField)
	}

	// try to get the create time, otherwise default to span start
	if createTimeStr, found := span.Meta[createTimeField]; found {
		ct, err := strconv.ParseInt(createTimeStr, 10, 64)
		if err != nil {
			return nil, err
		}
		createTime = ct
	} else {
		createTime = c.nanosToMillis(span.Start)
	}

	return &model.SpanMetadata{
		CreateTime: createTime,
		Hostname:   hostname,
		PID:        pid,
		Type:       span.Type,
		Kind:       kind,
	}, nil
}

func createSpanMetadataError(configField string) error {
	return errors.New(fmt.Sprintf("Field [%s] not found in Span", configField))
}
