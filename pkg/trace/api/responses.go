package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

const (
	receiverErrorKey = "datadog.trace_agent.receiver.error"
)

// We encaspulate the answers in a container, this is to ease-up transition,
// should we add another fied.
type traceResponse struct {
	// All the sampling rates recommended, by service
	Rates map[string]float64 `json:"rate_by_service"`
}

// HTTPFormatError is used for payload format errors
func HTTPFormatError(tags []string, w http.ResponseWriter) {
	tags = append(tags, "error:format-error")
	metrics.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, "format-error", http.StatusUnsupportedMediaType)
}

// HTTPDecodingError is used for errors happening in decoding
func HTTPDecodingError(err error, tags []string, w http.ResponseWriter) {
	status := http.StatusBadRequest
	errtag := "decoding-error"
	msg := err.Error()

	if err == ErrLimitedReaderLimitReached {
		status = http.StatusRequestEntityTooLarge
		errtag := "payload-too-large"
		msg = errtag
	}

	tags = append(tags, fmt.Sprintf("error:%s", errtag))
	metrics.Count(receiverErrorKey, 1, tags, 1)

	http.Error(w, msg, status)
}

// HTTPEndpointNotSupported is for payloads getting sent to a wrong endpoint
func HTTPEndpointNotSupported(tags []string, w http.ResponseWriter) {
	tags = append(tags, "error:unsupported-endpoint")
	metrics.Count(receiverErrorKey, 1, tags, 1)
	http.Error(w, "unsupported-endpoint", http.StatusInternalServerError)
}

// HTTPOK is a dumb response for when things are a OK
func HTTPOK(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK\n")
}

// HTTPRateByService outputs, as a JSON, the recommended sampling rates for all services.
func HTTPRateByService(w http.ResponseWriter, dynConf *sampler.DynamicConfig) {
	w.WriteHeader(http.StatusOK)
	response := traceResponse{
		Rates: dynConf.RateByService.GetAll(), // this is thread-safe
	}
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		tags := []string{"error:response-error"}
		metrics.Count(receiverErrorKey, 1, tags, 1)
		return
	}
}
