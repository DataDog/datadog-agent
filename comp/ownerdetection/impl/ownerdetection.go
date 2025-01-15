package impl

import (
	"context"
	"errors"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	ownerdetection "github.com/DataDog/datadog-agent/comp/ownerdetection/def"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	//tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
)

// Requires defines the dependencies of the tagger component.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Wmeta     workloadmeta.Component
	Telemetry coretelemetry.Component
	//Params    tagger.Params
}

// Provides contains the fields provided by the tagger constructor.
type Provides struct {
	compdef.Out

	Comp     ownerdetection.Component
	Endpoint api.AgentEndpointProvider
}

// datadogConfig contains the configuration specific to Dogstatsd.
type datadogConfig struct {
	// dogstatsdEntityIDPrecedenceEnabled disable enriching Dogstatsd metrics with tags from "origin detection" when Entity-ID is set.
	dogstatsdEntityIDPrecedenceEnabled bool
	// dogstatsdOptOutEnabled If enabled, and cardinality is none no origin detection is performed.
	dogstatsdOptOutEnabled bool
	// originDetectionUnifiedEnabled If enabled, all origin detection mechanisms will be unified to use the same logic.
	originDetectionUnifiedEnabled bool
}

// NewComponent returns a new owner detection client
func NewComponent(req Requires) (Provides, error) {

	cli, err := NewOwnerDetectionClient(req.Config, req.Wmeta, req.Log, req.Telemetry)
	if err != nil {
		return Provides{}, err
	}

	req.Log.Info("OwnerDetectionClient is created")
	req.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {
		// Main context passed to components, consistent with the one used in the workloadmeta component
		mainCtx, _ := common.GetMainCtxCancel()
		return cli.Start(mainCtx)
	}})
	req.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		return cli.Stop()
	}})

	return Provides{
		//Comp: nil,
		Endpoint: api.AgentEndpointProvider{},
	}, nil
}

// NewOwnerDetectionClient returns a new owner detection client
func NewOwnerDetectionClient(cfg config.Component, wmeta workloadmeta.Component, log log.Component, telemetryComp coretelemetry.Component) (ownerdetection.Component, error) {
	return &OwnerDetectionClient{
		wmeta:         wmeta,
		datadogConfig: datadogConfig{},
		log:           log,
	}, nil
}

type OwnerDetectionClient struct {
	wmeta         workloadmeta.Component
	datadogConfig datadogConfig
	log           log.Component
}

// Start calls defaultTagger.Start
func (odc *OwnerDetectionClient) Start(ctx context.Context) error {
	return errors.New("Not implemented")
}

// Stop calls defaultTagger.Stop
func (odc *OwnerDetectionClient) Stop() error {
	return errors.New("Not implemented")
}
