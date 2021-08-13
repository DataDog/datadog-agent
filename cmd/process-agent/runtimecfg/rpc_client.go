package runtimecfg

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

// Get retrieves the current value of a runtime setting from a running config client
func (p *ProcessAgentRuntimeConfigClient) Get(key string) (result interface{}, err error) {
	err = p.rpcClient.Call("RuntimeSettingRPCService.Get", key, &result)
	return
}

// Set assigns a runtime setting to a new value by calling a running config client
// It also returns whether the value you tried to set is hidden since this is required by the
// settings.Client interface.
func (p *ProcessAgentRuntimeConfigClient) Set(key string, value string) (hidden bool, err error) {
	err = p.rpcClient.Call("RuntimeSettingRPCService.Set", SetArg{key, value}, &hidden)
	return
}

// List retrieves all the runtime settings that the RPC Server understands. It lists their keys as well as description,
// and if they are hidden.
func (p *ProcessAgentRuntimeConfigClient) List() (map[string]settings.RuntimeSettingResponse, error) {
	result := make(map[string]settings.RuntimeSettingResponse)
	err := p.rpcClient.Call("RuntimeSettingRPCService.List", struct{}{}, &result)
	return result, err
}

// FullConfig lists the entire config in the process_config namespace.
// These settings reflect the config file that has been loaded at the start of the agent,
// and may not necessarily reflect runtime commands.
// As an added bonus, FullConfig's output should be valid yaml that can be loaded as a config file.
func (p *ProcessAgentRuntimeConfigClient) FullConfig() (string, error) {
	var result string
	err := p.rpcClient.Call("RuntimeSettingRPCService.FullConfig", struct{}{}, &result)
	return result, err
}
