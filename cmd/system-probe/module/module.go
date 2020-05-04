package module

import (
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
)

type Loader struct {
	modules map[string]Module
}

type Factory struct {
	Name string
	Fn   func(cfg *config.AgentConfig) (Module, error)
}

type Module interface {
	GetStats() map[string]interface{}
	Register(*grpc.Server) error
	Close()
}

func (l *Loader) Register(cfg *config.AgentConfig, server *grpc.Server, factories []Factory) error {
	for _, factory := range factories {
		module, err := factory.Fn(cfg)
		if err != nil {
			return err
		}

		if err = module.Register(server); err != nil {
			return err
		}

		l.modules[factory.Name] = module

		log.Infof("module: %s started", factory.Name)
	}

	return nil
}

func (l *Loader) Close() {
	for _, module := range l.modules {
		module.Close()
	}
}

func NewLoader() *Loader {
	return &Loader{
		modules: make(map[string]Module),
	}
}
