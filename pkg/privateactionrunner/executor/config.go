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

	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

func ControlPlaneConfig(config parconfig.Config, runner *parconfig.Identity) (*pb.GetControlPlaneConfigResponse, error) {
	response := &pb.GetControlPlaneConfigResponse{
		LogLevel: config.LogLevel(),
	}
	if runner == nil {
		return response, nil
	}
	if runner.IsIncomplete() {
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
	endpoint, err := config.APIBaseURL()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid OPMS endpoint")
	}
	proxyURL := ""
	proxy, err := config.ProxyURL(request)
	if err != nil {
		return nil, errors.New("failed to resolve OPMS proxy")
	}
	if proxy != nil {
		proxyURL = proxy.String()
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
		TaskConcurrency:   config.TaskConcurrency(),
		OpmsExtraHeaders:  config.OPMSExtraHeaders(),
		OpmsProxyUrl:      proxyURL,
		SkipSslValidation: config.SkipSSLValidation(),
		MinTlsVersion:     config.MinTLSVersion(),
	}
	return response, nil
}
