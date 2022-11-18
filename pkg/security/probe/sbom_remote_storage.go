// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"

	"github.com/golang/protobuf/proto"

	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	ddhttputil "github.com/DataDog/datadog-agent/pkg/util/http"
)

// SBOMRemoteStorage is a remote storage that forwards SBOMs to the backend
type SBOMRemoteStorage struct {
	urls        []string
	apiKeys     []string
	compression bool

	client *http.Client
}

// NewSBOMRemoteStorage returns a new instance of SBOMRemoteStorage
func NewSBOMRemoteStorage(enableCompression bool) (*SBOMRemoteStorage, error) {
	storage := &SBOMRemoteStorage{
		compression: enableCompression,
		client: &http.Client{
			Transport: ddhttputil.CreateHTTPTransport(),
		},
	}

	endpoints, err := config.SBOMRemoteStorageEndpoints("event-platform-intake.", "sbom", logsconfig.DefaultIntakeProtocol, "cloud-workload-security")
	if err != nil {
		return nil, fmt.Errorf("couldn't generate storage endpoints: %w", err)
	}
	for _, endpoint := range endpoints.GetReliableEndpoints() {
		storage.urls = append(storage.urls, utils.GetEndpointURL(endpoint, "api/v2/sbom"))
		storage.apiKeys = append(storage.apiKeys, endpoint.APIKey)
	}

	return storage, nil
}

func (storage *SBOMRemoteStorage) writeSBOM(writer io.Writer, sbom *api.SBOMMessage) error {
	encoded, err := proto.Marshal(sbom)
	if err != nil {
		return fmt.Errorf("couldn't encode SBOM: %w", err)
	}

	if _, err = writer.Write(encoded); err != nil {
		return fmt.Errorf("couldn't write SBOM to request body: %w", err)
	}
	return nil
}

func (storage *SBOMRemoteStorage) buildBody(sbom *api.SBOMMessage) (*bytes.Buffer, error) {
	body := bytes.NewBuffer(nil)
	var bodyWriter io.Writer

	if storage.compression {
		compressor := gzip.NewWriter(body)
		defer compressor.Close()
		bodyWriter = compressor
	} else {
		bodyWriter = body
	}

	if err := storage.writeSBOM(bodyWriter, sbom); err != nil {
		return nil, err
	}
	return body, nil
}

func (storage *SBOMRemoteStorage) sendToEndpoint(url string, apiKey string, body *bytes.Buffer) error {
	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body.Bytes()))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/x-protobuf")
	r.Header.Add("dd-api-key", apiKey)

	if storage.compression {
		r.Header.Set("Content-Encoding", "gzip")
	}

	resp, err := storage.client.Do(r)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	return fmt.Errorf(resp.Status)
}

// SendSBOM sends the provided SBOM to the remote endpoints
func (storage *SBOMRemoteStorage) SendSBOM(sbom *api.SBOMMessage) error {
	body, err := storage.buildBody(sbom)
	if err != nil {
		return fmt.Errorf("couldn't build request: %w", err)
	}

	for i, url := range storage.urls {
		if err = storage.sendToEndpoint(url, storage.apiKeys[i], body); err != nil {
			seclog.Warnf("couldn't sent SBOM to [%s, body size: %d]: %v", url, body.Len(), err)
		} else {
			seclog.Infof("SBOM for '%s - %s - %s' successfully sent to [%s]", sbom.GetHost(), sbom.GetContainerID(), sbom.GetTags(), url)
		}
	}

	return nil
}
