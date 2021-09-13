package inventories

import "github.com/DataDog/datadog-agent/pkg/config"

// SetConfigMetadata sets the agent metadata based on the given configuration
func SetConfigMetadata(cfg config.Config) {
	SetAgentMetadata(AgentConfigApmDDURL, cfg.GetString("apm_config.dd_url"))
	SetAgentMetadata(AgentConfigDDURL, cfg.GetString("dd_url"))
	SetAgentMetadata(AgentConfigLogsDDURL, cfg.GetString("logs_config.logs_dd_url"))
	SetAgentMetadata(AgentConfigLogsSocks5ProxyAddress, cfg.GetString("logs_config.socks5_proxy_address"))
	SetAgentMetadata(AgentConfigNoProxy, cfg.GetStringSlice("proxy.no_proxy"))
	SetAgentMetadata(AgentConfigProcessDDURL, cfg.GetString("process_config.process_dd_url"))
	SetAgentMetadata(AgentConfigProxyHTTP, cfg.GetString("proxy.http"))
	SetAgentMetadata(AgentConfigProxyHTTPS, cfg.GetString("proxy.https"))
}
