// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	ddhttputil "github.com/DataDog/datadog-agent/pkg/util/http"
)

// protobufFormat is `config.Protobuf.String()`, this is temporary duplication to not have
// to import the whole pkg/security/config package
const protobufFormat = "protobuf"

// ActivityDumpRemoteBackend is a remote backend that forwards dumps to the backend
type ActivityDumpRemoteBackend struct {
	endpoints        *logsconfig.Endpoints
	tooLargeEntities *atomic.Uint64

	client *http.Client
}

// NewActivityDumpRemoteBackend returns a new ActivityDumpRemoteBackend
func NewActivityDumpRemoteBackend() (*ActivityDumpRemoteBackend, error) {

	endpoints, err := activityDumpRemoteStorageEndpoints("cws-intake.", "secdump", logsconfig.DefaultIntakeProtocol, "cloud-workload-security")
	if err != nil {
		return nil, fmt.Errorf("couldn't generate storage endpoints: %w", err)
	}

	return &ActivityDumpRemoteBackend{
		tooLargeEntities: atomic.NewUint64(0),
		client: &http.Client{
			Transport: ddhttputil.CreateHTTPTransport(pkgconfigsetup.Datadog()),
		},
		endpoints: endpoints,
	}, nil
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
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="dump"; filename="dump.%s"`, protobufFormat))
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

	for _, endpoint := range backend.endpoints.Endpoints {
		url := utils.GetEndpointURL(endpoint, "api/v2/secdump")

		if err := backend.sendToEndpoint(url, endpoint.GetAPIKey(), writer, body); err != nil {
			seclog.Warnf("couldn't sent activity dump to [%s, body size: %d, dump size: %d]: %v", url, body.Len(), len(data), err)
		} else {
			seclog.Infof("[%s] file for activity dump [image_name:%s image_tag:%s] successfully sent to [%s]", protobufFormat, imageName, imageTag, url)
		}
	}

	return nil
}

// SendTelemetry sends telemetry for the current storage
func (backend *ActivityDumpRemoteBackend) SendTelemetry(sender statsd.ClientInterface) {
	// send too large entity metric
	tags := []string{"format:" + protobufFormat, "compression:true"}
	_ = sender.Count(metrics.MetricActivityDumpEntityTooLarge, int64(backend.tooLargeEntities.Load()), tags, 1.0)
}

// activityDumpRemoteStorageEndpoints returns the list of activity dump remote storage endpoints parsed from the agent config
func activityDumpRemoteStorageEndpoints(endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	logsConfig := logsconfig.NewLogsConfigKeys("runtime_security_config.activity_dump.remote_storage.endpoints.", pkgconfigsetup.Datadog())
	endpoints, err := logsconfig.BuildHTTPEndpointsWithConfig(pkgconfigsetup.Datadog(), logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		seclog.Infof("activity dump remote storage endpoint: %v\n", status)
	}
	return endpoints, nil
}

// GetEndpointsStatus returns the status of the endpoints
func (backend *ActivityDumpRemoteBackend) GetEndpointsStatus() []string {
	return backend.endpoints.GetStatus()
}
