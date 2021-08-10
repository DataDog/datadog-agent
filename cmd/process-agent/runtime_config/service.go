package runtime_config

import (
	"fmt"
	"github.com/cihub/seelog"
	"net"
	"net/http"
	"net/rpc"

	config2 "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Setting names
const (
	logLevel = "log_level"
)

type RuntimeSettingRPCService struct {
	agentConfig *config.AgentConfig
}

func getStringLogLevel() string {
	logLevel, err := log.GetLogLevel()
	if err != nil {
		log.Debug(err)
		return ""
	}
	switch logLevel {
	case seelog.TraceLvl:
		return "trace"
	case seelog.DebugLvl:
		return "debug"
	case seelog.InfoLvl:
		return "info"
	case seelog.WarnLvl:
		return "warn"
	case seelog.CriticalLvl:
		return "critical"
	case seelog.Off:
		return "off"
	default:
		panic("getStringLogLevel switch is not exhaustive") // This should never happen unless a log level is added
	}
}

func (svc *RuntimeSettingRPCService) Get(key *string, response *string) error {
	switch *key {
	case logLevel:
		*response = getStringLogLevel()
		break
	default:
		return fmt.Errorf("invalid setting: %v", key)
	}
	return nil
}

// SetArg is the arguments passed to the Set RPC command. The Key represents
// the name of the runtime setting to change, and the Value represents
// the Value you would like to change it to.
// There is definitely a much better way to do this, however the goal is to at least get things working first.
type SetArg struct {
	Key, Value string
}

func (svc *RuntimeSettingRPCService) Set(arg *SetArg, _ *struct{}) error {
	switch arg.Key {
	case logLevel:
		err := config2.ChangeLogLevel(arg.Value)
		if err != nil {
			return err
		}
		log.Info("log level now set to:", getStringLogLevel())
		break
	default:
		return fmt.Errorf("invalid setting: %v", arg.Key)
	}
	return nil
}

func (svc *RuntimeSettingRPCService) List(_ *struct{}, allSettings *map[string]settings.RuntimeSettingResponse) error {
	panic("implement me")
}

// StartRuntimeSettingRPCService Starts a runtime server and returns a reference to it
func StartRuntimeSettingRPCService(cfg *config.AgentConfig) (error, *RuntimeSettingRPCService) {
	svc := &RuntimeSettingRPCService{cfg}
	err := rpc.Register(svc)
	if err != nil {
		return err, nil
	}
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", ":1234")
	if err != nil {
		return err, nil
	}
	err = http.Serve(l, nil)
	if err != nil {
		return err, nil
	}
	log.Info("Runtime settings server listening on port 1234")
	return err, svc
}
