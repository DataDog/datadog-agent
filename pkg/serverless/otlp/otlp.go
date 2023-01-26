package otlp

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	coreOtlp "github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ServerlessOTLPAgent struct {
	pipeline *coreOtlp.Pipeline
}

func (o *ServerlessOTLPAgent) Start(serializer serializer.MetricSerializer) {
	var err error
	o.pipeline, err = coreOtlp.BuildAndStart(context.Background(), config.Datadog, serializer)
	if err != nil {
		log.Error("Error starting OTLP endpoint:", err)
	}
}

func (o *ServerlessOTLPAgent) Stop() {
	if o.pipeline != nil {
		o.pipeline.Stop()
	}
}

func IsEnabled() bool {
	return coreOtlp.IsEnabled(config.Datadog)
}

func (o *ServerlessOTLPAgent) IsReady() bool {
	if o.pipeline == nil {
		return false
	}
	if status := coreOtlp.GetCollectorStatus(o.pipeline); status.Status != "Running" {
		return false
	}
	return true
}

func (o *ServerlessOTLPAgent) Wait(timeout time.Duration) error {
	after := time.After(timeout)
	for {
		if o.IsReady() {
			return nil
		}
		select {
		case <-after:
			return fmt.Errorf("timeout waiting for otlp agent to start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
