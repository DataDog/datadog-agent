package runtime_config

import (
	"gopkg.in/yaml.v2"
	"net"
	"net/http"

	"net/rpc"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NOTE: Any settings you want to register can simply be added here
var processRuntimeSettings = []settings.RuntimeSetting{
	settings.LogLevelRuntimeSetting{},
}

type RuntimeSettingRPCService struct {
	agentConfig *config.AgentConfig
}

func (svc *RuntimeSettingRPCService) Get(key *string, settingResult *interface{}) error {
	setting, err := settings.GetRuntimeSetting(*key)
	if err != nil {
		return err
	}
	*settingResult = setting
	return nil
}

// SetArg is the arguments passed to the Set RPC command. The Key represents
// the name of the runtime setting to change, and the Value represents
// the Value you would like to change it to.
type SetArg struct {
	Key, Value string
}

func (svc *RuntimeSettingRPCService) Set(arg *SetArg, hidden *bool) error {
	err := settings.SetRuntimeSetting(arg.Key, arg.Value)
	if err != nil {
		return err
	}
	log.Infof("%s set to: %s", arg.Key, arg.Value)

	setting, ok := settings.RuntimeSettings()[arg.Key] // arg.Key is proven to exist, since we've already fetched it once.
	*hidden = ok && setting.Hidden()                   // BUT JUST IN CASE, we short circuit the value to false
	return nil
}

func (svc *RuntimeSettingRPCService) List(_ *struct{}, allSettings *map[string]settings.RuntimeSettingResponse) error {
	runtimeSettings := settings.RuntimeSettings()
	for _, setting := range runtimeSettings {
		(*allSettings)[setting.Name()] = settings.RuntimeSettingResponse{
			Description: setting.Description(),
			Hidden:      setting.Hidden(),
		}
	}
	return nil
}

func (svc *RuntimeSettingRPCService) FullConfig(_ *struct{}, result *string) error {
	// For some reason calling Get doesn't return the full namespace, so we have to do this
	fullConfig, ok := ddconfig.Datadog.AllSettings()["process_config"]
	if !ok {
		return nil
	}
	marshal, err := yaml.Marshal(fullConfig)
	if err != nil {
		return err
	}
	*result = string(marshal)
	return nil
}

func initRuntimeSettings() {
	for _, setting := range processRuntimeSettings {
		// RegisterRuntimeSetting only errors if there is a duplicate, which is fine, so we simply ignore the error.
		_ = settings.RegisterRuntimeSetting(setting)
	}
}

// StartRuntimeSettingRPCService Starts a runtime server and returns a reference to it
func StartRuntimeSettingRPCService(cfg *config.AgentConfig) (error, *RuntimeSettingRPCService) {
	initRuntimeSettings()

	svc := &RuntimeSettingRPCService{cfg}
	err := rpc.Register(svc)
	if err != nil {
		return err, nil
	}
	rpc.HandleHTTP()

	port := ddconfig.Datadog.GetString("process_config.config_port")
	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err, nil
	}

	go func() {
		if err := http.Serve(l, nil); err != nil {
			_ = log.Error(err)
		}
	}()

	log.Info("runtime settings server listening on port " + port)
	return err, svc
}
