// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package executor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func ControlPlaneConfig(config model.Reader, runner *parconfig.Config) (*pb.GetControlPlaneConfigResponse, error) {
	response := &pb.GetControlPlaneConfigResponse{
		LogLevel: config.GetString("log_level"),
	}
	if runner == nil {
		return response, nil
	}
	if runner.IdentityIsIncomplete() {
		return nil, errors.New("runner identity is incomplete")
	}
	jwk, err := util.EcdsaToJWK(runner.PrivateKey)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(jwk)
	if err != nil {
		return nil, err
	}
	endpoint := opms.EndpointURL(runner, "")
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid OPMS endpoint")
	}
	proxyURL := ""
	if proxies := config.GetProxies(); proxies != nil {
		proxy, err := httputils.GetProxyTransportFunc(proxies, config)(request)
		if err != nil {
			return nil, errors.New("failed to resolve OPMS proxy")
		}
		if proxy != nil {
			proxyURL = proxy.String()
		}
	}
	response.SplitMode = true
	response.Identity = &pb.ControlPlaneIdentity{
		Urn:        runner.Urn,
		PrivateKey: base64.RawURLEncoding.EncodeToString(encoded),
		OrgId:      runner.OrgId,
		RunnerId:   runner.RunnerId,
	}
	response.Runtime = &pb.ControlPlaneRuntime{
		OpmsBaseUrl:       endpoint,
		TaskConcurrency:   runner.RunnerPoolSize,
		OpmsExtraHeaders:  runner.OpmsExtraHeaders,
		OpmsProxyUrl:      proxyURL,
		SkipSslValidation: config.GetBool("skip_ssl_validation"),
		MinTlsVersion:     config.GetString("min_tls_version"),
	}
	return response, nil
}
