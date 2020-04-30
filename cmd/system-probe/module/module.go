package module

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
)

type Factory struct {
	modules map[string]Module
}

type Opts struct {
	GRPCServer *grpc.Server

	// add more options
	//FDServer *fdexporter.Server
}

type Caps struct {
}

type Spec struct {
	Name string
	Caps Caps
	New  func(cfg *config.AgentConfig, opts Opts) (Module, error)
}

type Status struct {
	LoadedMaps    []string
	EnableKProbes []string
	PerfRingMiss  int
}

type Module interface {
	// could be moved to grpc
	//GetStats() interface{}
	GetStatus() Status
	Start() error
	Stop()
}

func (f *Factory) Run(cfg *config.AgentConfig, specs []Spec, opts Opts) error {
	for _, spec := range specs {
		module, err := spec.New(cfg, opts)
		if err != nil {
			return err
		}

		f.modules[spec.Name] = module
	}

	for name, module := range f.modules {
		log.Infof("module: %s starting", name)
		if err := module.Start(); err != nil {
			return err
		}
		log.Infof("module: %s started", name)
	}

	return nil
}

func (f *Factory) Stop() {
	for _, module := range f.modules {
		module.Stop()
	}
}

func NewFactory() *Factory {
	return &Factory{
		modules: make(map[string]Module),
	}
}
