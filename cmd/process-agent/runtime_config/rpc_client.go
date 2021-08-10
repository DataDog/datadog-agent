package runtime_config

import (
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"net/rpc"
)

type ProcessAgentRuntimeConfigClient struct {
	rpcClient *rpc.Client
}

func SetupConfig() (cli settings.Client, err error) {
	rpcClient, err := rpc.DialHTTP("tcp", "localhost:1234")
	cli = &ProcessAgentRuntimeConfigClient{rpcClient}
	return
}

func (p *ProcessAgentRuntimeConfigClient) Get(key string) (interface{}, error) {
	var value string
	err := p.rpcClient.Call("RuntimeSettingRPCService.Get", &key, &value)
	return value, err
}

func (p *ProcessAgentRuntimeConfigClient) Set(key string, value string) (bool, error) {
	err := p.rpcClient.Call("RuntimeSettingRPCService.Set", &SetArg{key, value}, nil)

	// Kind of pointless, but necessary for this method to implement the settings.Client interface
	isSet := err == nil

	return isSet, err
}

func (p *ProcessAgentRuntimeConfigClient) List() (map[string]settings.RuntimeSettingResponse, error) {
	panic("implement me")
}

func (p *ProcessAgentRuntimeConfigClient) FullConfig() (string, error) {
	panic("implement me")
}
