// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type IntegrationConfigProvider interface {
	GetIntegrationConfigs(context.Context) ([]integration.Config, error)
}

type agentIntegrationConfigProvider struct {
	client ipc.HTTPClient
}

func NewAgentIntegrationConfigProvider(client ipc.HTTPClient) IntegrationConfigProvider {
	return &agentIntegrationConfigProvider{client: client}
}

func (p *agentIntegrationConfigProvider) GetIntegrationConfigs(ctx context.Context) ([]integration.Config, error) {
	if p.client == nil {
		return nil, errors.New("Agent IPC client is unavailable")
	}
	endpoint, err := p.client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return nil, fmt.Errorf("create Agent config-check endpoint: %w", err)
	}
	values := url.Values{}
	values.Set("raw", "true")
	body, err := endpoint.DoGet(ipchttp.WithContext(ctx), ipchttp.WithValues(values))
	if err != nil {
		return nil, fmt.Errorf("get active integration configurations: %w", err)
	}

	var response integration.ConfigCheckResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode active integration configurations: %w", err)
	}
	configs := make([]integration.Config, 0, len(response.Configs))
	for _, item := range response.Configs {
		configs = append(configs, item.Config)
	}
	return configs, nil
}

func resolveIntegrationCredentials(ctx context.Context, provider IntegrationConfigProvider, selection *privateactionspb.IntegrationCredential) (map[string]string, error) {
	if provider == nil {
		return nil, errors.New("integration credentials are disabled")
	}
	if selection == nil || selection.GetIntegration() == "" {
		return nil, errors.New("integration credential must specify an integration")
	}

	configs, err := provider.GetIntegrationConfigs(ctx)
	if err != nil {
		return nil, err
	}

	var matches []map[string]any
	for _, config := range configs {
		if config.Name != selection.GetIntegration() {
			continue
		}
		for _, rawInstance := range config.Instances {
			instance := make(map[string]any)
			if err := yaml.Unmarshal(rawInstance, &instance); err != nil {
				return nil, fmt.Errorf("decode %q integration instance: %w", config.Name, err)
			}
			if instanceMatches(instance, selection.GetSelectors()) {
				matches = append(matches, instance)
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, errors.New("requested integration credential is not available")
	case 1:
		return credentialsFromIntegrationInstance(selection.GetIntegration(), matches[0]), nil
	default:
		return nil, errors.New("integration credential selector matched more than one instance")
	}
}

func instanceMatches(instance map[string]any, selectors map[string]string) bool {
	for path, expected := range selectors {
		value, found := selectorValue(instance, path)
		if !found {
			return false
		}
		actual, ok := scalarString(value)
		if !ok || actual != expected {
			return false
		}
	}
	return true
}

func selectorValue(instance map[string]any, path string) (any, bool) {
	value, found := valueAtPath(instance, strings.Split(path, "."))
	if found || strings.Contains(path, ".") {
		return value, found
	}
	aliases := map[string][]string{
		"host":     {"hostname"},
		"hostname": {"host"},
		"user":     {"username"},
		"username": {"user"},
		"database": {"dbname", "database_name"},
	}
	for _, alias := range aliases[path] {
		if value, found := instance[alias]; found {
			return value, true
		}
	}
	return nil, false
}

func valueAtPath(value any, path []string) (any, bool) {
	current := value
	for _, segment := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func scalarString(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return value, true
	case bool:
		return strconv.FormatBool(value), true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case uint64:
		return strconv.FormatUint(value, 10), true
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64), true
	default:
		return "", false
	}
}

func credentialsFromIntegrationInstance(integrationName string, instance map[string]any) map[string]string {
	credentials := make(map[string]string)
	for key, value := range instance {
		if scalar, ok := scalarString(value); ok {
			credentials[key] = scalar
		}
	}

	copyFirst(credentials, "host", "hostname")
	copyFirst(credentials, "username", "user")
	copyFirst(credentials, "database", "dbname", "database_name")
	copyFirst(credentials, "authSource", "auth_source")
	copyFirst(credentials, "authMechanism", "auth_mechanism")
	copyFirst(credentials, "tls", "ssl")
	addHostPortFromList(credentials, instance["hosts"])
	addCredentialsFromServerURI(credentials, instance["server"])

	if credentials["port"] == "" {
		credentials["port"] = defaultIntegrationPort(integrationName)
	}
	return credentials
}

func copyFirst(credentials map[string]string, destination string, sources ...string) {
	if credentials[destination] != "" {
		return
	}
	for _, source := range sources {
		if credentials[source] != "" {
			credentials[destination] = credentials[source]
			return
		}
	}
}

func addHostPortFromList(credentials map[string]string, value any) {
	if credentials["host"] != "" {
		return
	}
	values, ok := value.([]any)
	if !ok || len(values) != 1 {
		return
	}
	hostPort, ok := values[0].(string)
	if !ok {
		return
	}
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		credentials["host"] = hostPort
		return
	}
	credentials["host"] = host
	credentials["port"] = port
}

func addCredentialsFromServerURI(credentials map[string]string, value any) {
	server, ok := value.(string)
	if !ok || !strings.Contains(server, "://") {
		return
	}
	parsed, err := url.Parse(server)
	if err != nil {
		return
	}
	if credentials["host"] == "" {
		credentials["host"] = parsed.Hostname()
	}
	if credentials["port"] == "" {
		credentials["port"] = parsed.Port()
	}
	if parsed.User != nil {
		if credentials["username"] == "" {
			credentials["username"] = parsed.User.Username()
		}
		if password, found := parsed.User.Password(); found && credentials["password"] == "" {
			credentials["password"] = password
		}
	}
	if credentials["database"] == "" {
		credentials["database"] = strings.TrimPrefix(parsed.Path, "/")
	}
	copyQueryValue(credentials, parsed.Query(), "authSource")
	copyQueryValue(credentials, parsed.Query(), "authMechanism")
	copyQueryValue(credentials, parsed.Query(), "tls")
}

func copyQueryValue(credentials map[string]string, values url.Values, key string) {
	if credentials[key] == "" && values.Get(key) != "" {
		credentials[key] = values.Get(key)
	}
}

func defaultIntegrationPort(integrationName string) string {
	switch integrationName {
	case "postgres":
		return "5432"
	case "mysql":
		return "3306"
	case "sqlserver":
		return "1433"
	case "mongo":
		return "27017"
	case "oracle":
		return "1521"
	default:
		return ""
	}
}
