// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package backend holds files related to forwarder backends for security profiles
package backend

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"go.uber.org/atomic"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	ddhttputil "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// ActivityDumpRemoteBackend is a remote backend that forwards dumps to the backend
type ActivityDumpRemoteBackend struct {
	endpoints        []remoteEndpoint
	tooLargeEntities *atomic.Uint64

	client *http.Client
}

type remoteEndpoint struct {
	logsEndpoint logsconfig.Endpoint
	url          string
}

// NewActivityDumpRemoteBackend returns a new ActivityDumpRemoteBackend
func NewActivityDumpRemoteBackend() (*ActivityDumpRemoteBackend, error) {
	backend := &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client: &http.Client{
			Transport: ddhttputil.CreateHTTPTransport(pkgconfigsetup.Datadog()),
		},
	}

	endpoints, err := config.ActivityDumpRemoteStorageEndpoints("cws-intake.", "secdump", logsconfig.DefaultIntakeProtocol, "cloud-workload-security")
	if err != nil {
		return nil, fmt.Errorf("couldn't generate storage endpoints: %w", err)
	}
	for _, endpoint := range endpoints.GetReliableEndpoints() {
		backend.endpoints = append(backend.endpoints, remoteEndpoint{
			logsEndpoint: endpoint,
			url:          utils.GetEndpointURL(endpoint, "api/v2/secdump"),
		})
	}

	return backend, nil
}

func writeEventMetadata(writer *multipart.Writer, header []byte) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="event"; filename=""`)
	h.Set("Content-Type", "application/json")

	dataWriter, err := writer.CreatePart(h)
	if err != nil {
		return fmt.Errorf("couldn't create event metadata part: %w", err)
	}

	// write metadata
	if _, err = dataWriter.Write(header); err != nil {
		return fmt.Errorf("couldn't write event metadata part: %w", err)
	}
	return err
}

func writeDump(writer *multipart.Writer, raw []byte) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="dump"; filename="dump.%s"`, config.Protobuf))
	h.Set("Content-Type", "application/json")

	dataWriter, err := writer.CreatePart(h)
	if err != nil {
		return fmt.Errorf("couldn't create dump part: %w", err)
	}
	if _, err = dataWriter.Write(raw); err != nil {
		return fmt.Errorf("couldn't write dump part: %w", err)
	}
	return nil
}

func buildBody(header []byte, data []byte) (*multipart.Writer, *bytes.Buffer, error) {
	body := bytes.NewBuffer(nil)
	var multipartWriter *multipart.Writer

	compressor := gzip.NewWriter(body)
	defer compressor.Close()

	multipartWriter = multipart.NewWriter(compressor)
	defer multipartWriter.Close()

	if err := writeEventMetadata(multipartWriter, header); err != nil {
		return nil, nil, err
	}

	if err := writeDump(multipartWriter, data); err != nil {
		return nil, nil, err
	}
	return multipartWriter, body, nil
}

func (backend *ActivityDumpRemoteBackend) sendToEndpoint(url string, apiKey string, writer *multipart.Writer, body *bytes.Buffer) error {
	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body.Bytes()))
	if err != nil {
		return err
	}
	r.Header.Add("Content-Type", writer.FormDataContentType())
	r.Header.Add("dd-api-key", apiKey)

	if /*request.Compression*/ true {
		r.Header.Set("Content-Encoding", "gzip")
	}

	resp, err := backend.client.Do(r)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		backend.tooLargeEntities.Inc()
	}
	return errors.New(resp.Status)
}

// HandleActivityDump sends the activity dump to the remote backend
func (backend *ActivityDumpRemoteBackend) HandleActivityDump(imageName string, imageTag string, header []byte, data []byte) error {
	writer, body, err := buildBody(header, data)
	if err != nil {
		return fmt.Errorf("couldn't build request: %w", err)
	}

	selector := &cgroupModel.WorkloadSelector{
		Image: imageName,
		Tag:   imageTag,
	}

	for _, endpoint := range backend.endpoints {
		if err := backend.sendToEndpoint(endpoint.url, endpoint.logsEndpoint.GetAPIKey(), writer, body); err != nil {
			seclog.Warnf("couldn't sent activity dump to [%s, body size: %d, dump size: %d]: %v", endpoint.url, body.Len(), len(data), err)
		} else {
			seclog.Infof("[%s] file for activity dump [%s] successfully sent to [%s]", config.Protobuf, selector, endpoint.url)
		}
	}

	return nil
}

// SendTelemetry sends telemetry for the current storage
func (backend *ActivityDumpRemoteBackend) SendTelemetry(sender statsd.ClientInterface) {
	// send too large entity metric
	tags := []string{fmt.Sprintf("format:%s", config.Protobuf), fmt.Sprintf("compression:%v", true)}
	_ = sender.Count(metrics.MetricActivityDumpEntityTooLarge, int64(backend.tooLargeEntities.Load()), tags, 1.0)
}
