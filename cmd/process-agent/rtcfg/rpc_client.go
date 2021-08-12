package rtcfg

import (
	"net/rpc"

	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

// ProcessAgentRuntimeConfigClient is a client designed to consume the RPC endpoints opened by RuntimeSettingRPCService.
// It implements the settings.Client interface, so that the process-agent cli can act similar to the core agent's.
type ProcessAgentRuntimeConfigClient struct {
	rpcClient *rpc.Client
}

// NewProcessAgentRuntimeConfigClient creates a new client and opens a connection to the RPC endpoint created by RuntimeSettingRPCService.
func NewProcessAgentRuntimeConfigClient(port string) (*ProcessAgentRuntimeConfigClient, error) {
	rpcClient, err := rpc.DialHTTP("tcp", "localhost:"+port)
	if err != nil {
		return nil, err
	}
	return &ProcessAgentRuntimeConfigClient{rpcClient}, nil
}

func (p *ProcessAgentRuntimeConfigClient) Get(key string) (result interface{}, err error) {
	err = p.rpcClient.Call("RuntimeSettingRPCService.Get", key, &result)
	return
}

func (p *ProcessAgentRuntimeConfigClient) Set(key string, value string) (hidden bool, err error) {
	err = p.rpcClient.Call("RuntimeSettingRPCService.Set", SetArg{key, value}, &hidden)
	return
}

func (p *ProcessAgentRuntimeConfigClient) List() (map[string]settings.RuntimeSettingResponse, error) {
	result := make(map[string]settings.RuntimeSettingResponse)
	err := p.rpcClient.Call("RuntimeSettingRPCService.List", struct{}{}, &result)
	return result, err
}

func (p *ProcessAgentRuntimeConfigClient) FullConfig() (string, error) {
	var result string
	err := p.rpcClient.Call("RuntimeSettingRPCService.FullConfig", struct{}{}, &result)
	return result, err
}
