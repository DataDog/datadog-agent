// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"go.opentelemetry.io/collector/confmap"
)

// Config defines configuration for trace-agent
type Config struct {
	agentConf    *config.AgentConfig
	API          APIConfig `mapstructure:"api"`
	ReceiverHost string    `mapstructure:"host"`
}

// APIConfig defines the API configuration options
type APIConfig struct {
	// Key is the Datadog API key to associate your Agent's data with your organization.
	// Create a new API key here: https://app.datadoghq.com/account/settings
	Key string `mapstructure:"key"`

	// Site is the site of the Datadog intake to send data to.
	// The default value is "datadoghq.com".
	Site string `mapstructure:"site"`

	// FailOnInvalidKey states whether to exit at startup on invalid API key.
	// The default value is false.
	FailOnInvalidKey bool `mapstructure:"fail_on_invalid_key"`
}

var _ confmap.Unmarshaler = (*Config)(nil)

// Unmarshal a configuration map into the configuration struct.
func (c *Config) Unmarshal(configMap *confmap.Conf) error {

	err := configMap.Unmarshal(c, confmap.WithErrorUnused())
	if err != nil {
		return err
	}

	c.agentConf.Endpoints[0].APIKey = c.API.Key
	c.agentConf.ReceiverHost = c.ReceiverHost

	return nil
}
