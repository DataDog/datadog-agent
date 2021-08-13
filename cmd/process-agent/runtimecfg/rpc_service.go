package runtimecfg

import (
	"gopkg.in/yaml.v2"
	"net"
	"net/http"
	"net/rpc"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NOTE: Any settings you want to register should simply be added here
var processRuntimeSettings = []settings.RuntimeSetting{
	settings.LogLevelRuntimeSetting{},
}

// RuntimeSettingRPCService is a service designed to allow runtime settings to be changed via a client.
// It is designed to be consumed by a ProcessAgentRuntimeConfigClient.
type RuntimeSettingRPCService struct{}

// Get is an RPC endpoint that retrieves a runtime setting.
// The function prototype may look a bit weird, however this is because RPC endpoints have special rules.
// See https://pkg.go.dev/net/rpc.
func (svc *RuntimeSettingRPCService) Get(key string, settingResult *interface{}) error {
	setting, err := settings.GetRuntimeSetting(key)
	if err != nil {
		return err
	}
	*settingResult = setting
	return nil
}

// SetArg is the arguments passed to the Set RPC command. The Key represents
// the name of the runtime setting to change, and the Value represents what it should be changed to
type SetArg struct {
	Key, Value string
}

// Set is an RPC endpoint that changes a runtime setting.
// It also tells the caller whether the variable is hidden, because the interface in settings.Client requires it.
// The function prototype may look a bit weird, however this is because RPC endpoints have special rules.
// See https://pkg.go.dev/net/rpc.
func (svc *RuntimeSettingRPCService) Set(arg SetArg, hidden *bool) error {
	err := settings.SetRuntimeSetting(arg.Key, arg.Value)
	if err != nil {
		return err
	}
	log.Infof("%s set to: %s", arg.Key, arg.Value)

	setting, ok := settings.RuntimeSettings()[arg.Key] // arg.Key is proven to exist, since we've already fetched it once.
	*hidden = ok && setting.Hidden()                   // BUT JUST IN CASE, we short circuit the value to false
	return nil
}

// List is an RPC endpoint that returns a map from the names of the registered runtime settings to their actual structs.
// The function prototype may look a bit weird, however this is because RPC endpoints have special rules.
// See https://pkg.go.dev/net/rpc.
func (svc *RuntimeSettingRPCService) List(_ struct{}, allSettings *map[string]settings.RuntimeSettingResponse) error {
	runtimeSettings := settings.RuntimeSettings()
	for _, setting := range runtimeSettings {
		(*allSettings)[setting.Name()] = settings.RuntimeSettingResponse{
			Description: setting.Description(),
			Hidden:      setting.Hidden(),
		}
	}
	return nil
}

// FullConfig is an RPC endpoint that returns all the settings in the process_config namespace in a string.
// The caller can expect the result to be valid yaml.
// The function prototype may look a bit weird, however this is because RPC endpoints have special rules.
// See https://pkg.go.dev/net/rpc.
func (svc *RuntimeSettingRPCService) FullConfig(_ struct{}, result *string) error {
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

// StartRuntimeSettingRPCService starts up a runtime setting rpc service meant to be consumed by a ProcessAgentRuntimeConfigClient.
func StartRuntimeSettingRPCService(port string) error {
	initRuntimeSettings()

	svc := &RuntimeSettingRPCService{}
	err := rpc.Register(svc)
	if err != nil {
		return err
	}
	rpc.HandleHTTP()

	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// http.Serve is blocking, so we need to put it in a goroutine
	go func() {
		if err := http.Serve(l, nil); err != nil {
			_ = log.Error(err)
		}
	}()

	log.Info("runtime settings server listening on port " + port)
	return nil
}
