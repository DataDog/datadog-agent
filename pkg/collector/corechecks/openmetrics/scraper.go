// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type labelExcluder func(string) bool

type openmetricsScraper struct {
	cfg           *scraperConfig
	httpClient    *http.Client
	rawLineFilter *regexp.Regexp

	excludeMetrics        map[string]struct{}
	excludeMetricPatterns *regexp.Regexp
	labelExcluders        map[string]labelExcluder

	includeLabels map[string]struct{}
	excludeLabels map[string]struct{}

	transformer     *metricTransformer
	labelAggregator *labelAggregator

	staticTags []string
	tags       []string

	flushFirstValue bool

	bearerTokenMu        sync.Mutex
	cachedBearerToken    string
	bearerTokenRefreshed time.Time
}

type scrapePrepass struct {
	processStartTime float64
}

type scrapeResponse struct {
	body          io.ReadCloser
	contentType   string
	contentLength int64
}

var defaultBearerTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

const maxPreallocatedBodySize = 1024 * 1024

func newScraper(cfg *scraperConfig) (*openmetricsScraper, error) {
	rawLineFilter, err := compileRegexList(cfg.rawLineFilters)
	if err != nil {
		return nil, err
	}

	exactExclude, patternExclude, err := compileExcludeMetrics(cfg.excludeMetrics)
	if err != nil {
		return nil, err
	}
	labelExcluders, err := compileLabelExcluders(cfg.excludeMetricsByLabels)
	if err != nil {
		return nil, err
	}
	transformer, err := newMetricTransformer(cfg)
	if err != nil {
		return nil, err
	}
	labelAggregator, err := newLabelAggregator(cfg)
	if err != nil {
		return nil, err
	}
	staticTags, err := staticTags(cfg)
	if err != nil {
		return nil, err
	}
	httpClient, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	scraper := &openmetricsScraper{
		cfg:                   cfg,
		httpClient:            httpClient,
		rawLineFilter:         rawLineFilter,
		excludeMetrics:        exactExclude,
		excludeMetricPatterns: patternExclude,
		labelExcluders:        labelExcluders,
		includeLabels:         stringSet(cfg.includeLabels),
		excludeLabels:         stringSet(cfg.excludeLabels),
		transformer:           transformer,
		labelAggregator:       labelAggregator,
		staticTags:            staticTags,
		tags:                  staticTags,
	}
	return scraper, nil
}

func (s *openmetricsScraper) close() {
	if s != nil && s.httpClient != nil {
		s.httpClient.CloseIdleConnections()
	}
}

func (s *openmetricsScraper) scrape(sender sender.Sender) error {
	response, err := s.fetch(sender)
	if err != nil {
		if s.flushFirstValue {
			s.flushFirstValue = false
		}
		return err
	}
	if response == nil {
		s.flushFirstValue = true
		return nil
	}
	defer response.body.Close()

	useOpenMetrics := s.cfg.useLatestSpec || responseUsesOpenMetrics(response.contentType)
	if s.canDirectStreamParse(useOpenMetrics) {
		return s.scrapeDirectStream(sender, response)
	}
	if s.canOpenMetricsStreamParse(useOpenMetrics) {
		return s.scrapeOpenMetricsStream(sender, response)
	}

	body, readErr := readResponseBody(response)
	if readErr != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, readErr.Error())
		if s.flushFirstValue {
			s.flushFirstValue = false
		}
		return readErr
	}
	prepass := scrapePrepass{processStartTime: math.Inf(1)}
	if s.needsPrepass() {
		if _, err := s.runPrepass(body, useOpenMetrics, &prepass); err != nil {
			if s.flushFirstValue {
				s.flushFirstValue = false
			}
			return err
		}
	}
	runtime := runtimeData{
		flushFirstValue: s.flushFirstValue || s.shouldFlushFirstValue(prepass.processStartTime),
		staticTags:      s.staticTags,
	}

	submittedSamples := 0
	ignoredLines, err := walkParsedMetrics(body, s.rawLineFilter, useOpenMetrics, s.cfg.mode == latestMode, func(metric parsedMetric) (bool, error) {
		metric = s.applyRawMetricPrefix(metric)
		count := s.processMetric(metric, runtime, sender, submittedSamples)
		submittedSamples += count
		keepMaterializing := s.cfg.maxReturnedMetrics <= 0 || submittedSamples < s.cfg.maxReturnedMetrics
		return keepMaterializing, nil
	})
	if err != nil {
		if s.flushFirstValue {
			s.flushFirstValue = false
		}
		return err
	}

	if s.cfg.telemetry {
		s.submitTelemetryCount(sender, "telemetry.metrics.blacklist.count", float64(ignoredLines))
		s.submitTelemetryGauge(sender, "telemetry.payload.size", s.payloadSize(response, len(body)))
	}

	s.labelAggregator.afterScrape()
	s.flushFirstValue = true
	return nil
}

func readResponseBody(response *scrapeResponse) ([]byte, error) {
	if response.contentLength > 0 && response.contentLength <= maxPreallocatedBodySize {
		var buffer bytes.Buffer
		buffer.Grow(int(response.contentLength))
		_, err := buffer.ReadFrom(response.body)
		return buffer.Bytes(), err
	}
	return io.ReadAll(response.body)
}

func (s *openmetricsScraper) canDirectStreamParse(useOpenMetrics bool) bool {
	return !useOpenMetrics && s.canTextFastPath()
}

func (s *openmetricsScraper) canOpenMetricsStreamParse(useOpenMetrics bool) bool {
	return useOpenMetrics && s.canTextFastPath()
}

func (s *openmetricsScraper) canTextFastPath() bool {
	return s.cfg.mode == latestMode &&
		s.cfg.maxReturnedMetrics > 0 &&
		!s.cfg.telemetry &&
		s.rawLineFilter == nil &&
		!s.needsPrepass() &&
		s.cfg.rawMetricPrefix == "" &&
		s.hasOnlyDirectStreamCompatibleTransformers() &&
		len(s.transformer.patterns) == 0 &&
		len(s.excludeMetrics) == 0 &&
		s.excludeMetricPatterns == nil &&
		len(s.labelExcluders) == 0 &&
		len(s.includeLabels) == 0 &&
		len(s.excludeLabels) == 0 &&
		len(s.cfg.renameLabels) == 0 &&
		s.cfg.hostnameLabel == "" &&
		!s.labelAggregator.configured &&
		s.cfg.collectHistogramBuckets &&
		!s.cfg.nonCumulativeHistogramBuckets &&
		!s.cfg.histogramBucketsAsDistributions &&
		!s.cfg.collectCountersWithDistributions
}

func (s *openmetricsScraper) hasOnlyDirectStreamCompatibleTransformers() bool {
	if len(s.transformer.exact) == 0 {
		return false
	}
	for _, transformer := range s.transformer.exact {
		if !directStreamCompatibleTransformer(transformer.metricType) {
			return false
		}
	}
	return true
}

func directStreamCompatibleTransformer(metricType string) bool {
	switch metricType {
	case transformerNative, "counter", "gauge", "histogram", "summary":
		return true
	default:
		return false
	}
}

func (s *openmetricsScraper) scrapeDirectStream(sender sender.Sender, response *scrapeResponse) error {
	runtime := runtimeData{
		flushFirstValue: s.flushFirstValue,
		staticTags:      s.staticTags,
	}
	submittedSamples := 0
	result, err := s.walkAndSubmitPrometheusTextStream(response.body, runtime, sender, &submittedSamples)
	if err != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		if s.flushFirstValue {
			s.flushFirstValue = false
		}
		return err
	}
	if s.cfg.telemetry {
		s.submitTelemetryCount(sender, "telemetry.metrics.blacklist.count", float64(result.ignoredLines))
		s.submitTelemetryGauge(sender, "telemetry.payload.size", s.payloadSize(response, int(result.bytesRead)))
	}

	s.labelAggregator.afterScrape()
	s.flushFirstValue = true
	return nil
}

func (s *openmetricsScraper) scrapeOpenMetricsStream(sender sender.Sender, response *scrapeResponse) error {
	runtime := runtimeData{
		flushFirstValue: s.flushFirstValue,
		staticTags:      s.staticTags,
	}
	submittedSamples := 0
	samples := make([]parsedSample, 1)
	metric := parsedMetric{Samples: samples}
	result, err := walkOpenMetricsTextSamples(response.body, s.cfg.mode == latestMode, func(familyName string) bool {
		if s.cfg.maxReturnedMetrics > 0 && submittedSamples >= s.cfg.maxReturnedMetrics {
			return false
		}
		_, configured := s.transformer.exact[familyName]
		return configured
	}, func(sample parsedSample, metricFamily parsedMetric) error {
		samples[0] = sample
		metric.Name = metricFamily.Name
		metric.Type = metricFamily.Type
		submittedSamples += s.processMetric(metric, runtime, sender, submittedSamples)
		return nil
	})
	if err != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		if s.flushFirstValue {
			s.flushFirstValue = false
		}
		return err
	}
	if s.cfg.telemetry {
		s.submitTelemetryCount(sender, "telemetry.metrics.blacklist.count", float64(result.ignoredLines))
		s.submitTelemetryGauge(sender, "telemetry.payload.size", s.payloadSize(response, int(result.bytesRead)))
	}

	s.labelAggregator.afterScrape()
	s.flushFirstValue = true
	return nil
}

func (s *openmetricsScraper) walkAndSubmitPrometheusTextStream(r io.Reader, runtime runtimeData, sender sender.Sender, submittedSamples *int) (streamParseResult, error) {
	shouldMaterialize := func(sampleName []byte, metricTypes map[string]string) bool {
		if s.cfg.maxReturnedMetrics > 0 && *submittedSamples >= s.cfg.maxReturnedMetrics {
			return false
		}
		familyName := prometheusFamilyNameBytes(sampleName, metricTypes, s.cfg.mode == latestMode)
		_, configured := s.transformer.exact[familyName]
		return configured
	}
	return walkPrometheusTextSamples(r, s.cfg.mode == latestMode, shouldMaterialize, func(sample parsedSample, metricTypes map[string]string) error {
		familyName := prometheusFamilyName(sample.Name, metricTypes, s.cfg.mode == latestMode)
		metric := parsedMetric{
			Name:    familyName,
			Type:    metricTypes[familyName],
			Samples: []parsedSample{sample},
		}
		if metric.Type == "" {
			metric.Type = "unknown"
		}
		*submittedSamples += s.processMetric(metric, runtime, sender, *submittedSamples)
		return nil
	})
}

func (s *openmetricsScraper) needsPrepass() bool {
	return s.labelAggregator.needsPrepass() || (s.cfg.useProcessStartTime && !s.flushFirstValue)
}

func (s *openmetricsScraper) runPrepass(data []byte, useOpenMetrics bool, prepass *scrapePrepass) (int, error) {
	var labelPreparer *labelAggregatorPreparer
	if s.labelAggregator.needsPrepass() {
		labelPreparer = s.labelAggregator.newPreparer()
	}

	needProcessStartTime := s.cfg.useProcessStartTime && !s.flushFirstValue
	labelPrepassDone := labelPreparer == nil
	return walkParsedMetrics(data, s.rawLineFilter, useOpenMetrics, s.cfg.mode == latestMode, func(metric parsedMetric) (bool, error) {
		metric = s.applyRawMetricPrefix(metric)
		if labelPreparer != nil {
			labelPrepassDone = labelPreparer.collect(metric)
		}
		if metric.Name != "process_start_time_seconds" {
			return !labelPrepassDone || needProcessStartTime, nil
		}
		for _, sample := range metric.Samples {
			if sample.Value < prepass.processStartTime {
				prepass.processStartTime = sample.Value
			}
		}
		return !labelPrepassDone || needProcessStartTime, nil
	})
}

func (s *openmetricsScraper) shouldFlushFirstValue(processStart float64) bool {
	if !s.cfg.useProcessStartTime || s.flushFirstValue {
		return false
	}
	agentStart := float64(pkgconfigsetup.StartTime.Unix())
	return processStart < math.Inf(1) && processStart > agentStart
}

func (s *openmetricsScraper) processMetric(metric parsedMetric, runtime runtimeData, sender sender.Sender, submittedSamples int) int {
	if s.cfg.maxReturnedMetrics > 0 && submittedSamples >= s.cfg.maxReturnedMetrics {
		return 0
	}
	s.labelAggregator.beforeMetric(metric)
	if s.isMetricExcluded(metric) {
		if s.cfg.telemetry {
			s.submitTelemetryCount(sender, "telemetry.metrics.ignored.count", float64(len(metric.Samples)))
		}
		return 0
	}
	if s.cfg.telemetry {
		s.submitTelemetryCount(sender, "telemetry.metrics.input.count", float64(len(metric.Samples)))
	}

	transformer := s.transformer.get(metric)
	if transformer == nil {
		return 0
	}
	samples := s.generateSampleData(metric, sender)
	if s.cfg.maxReturnedMetrics > 0 {
		remaining := s.cfg.maxReturnedMetrics - submittedSamples
		if remaining <= 0 {
			return 0
		}
		if len(samples) > remaining {
			samples = samples[:remaining]
		}
	}
	transformer.transformer(metric, samples, runtime, sender)
	return len(samples)
}

func (s *openmetricsScraper) fetch(sender sender.Sender) (*scrapeResponse, error) {
	request, err := http.NewRequest(http.MethodGet, s.cfg.endpoint, nil)
	if err != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		return nil, err
	}

	for key, value := range s.cfg.headers {
		if strings.EqualFold(key, "Host") {
			request.Host = value
		}
		request.Header.Set(key, value)
	}
	if request.Header.Get("Accept") == "" || request.Header.Get("Accept") == "*/*" {
		if s.cfg.useLatestSpec {
			request.Header.Set("Accept", "application/openmetrics-text;version=1.0.0,application/openmetrics-text;version=0.0.1")
		} else {
			request.Header.Set("Accept", "text/plain")
		}
	}
	if s.cfg.basicAuthConfigured {
		authorization, err := basicAuthorization(s.cfg.username, s.cfg.password, s.cfg.legacyAuthEncoding)
		if err != nil {
			s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
			return nil, err
		}
		request.Header.Set("Authorization", authorization)
	}
	if s.cfg.bearerTokenAuth {
		token, err := s.bearerToken()
		if err != nil {
			s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if err := s.cfg.authToken.apply(request); err != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		return nil, err
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		if s.cfg.ignoreConnectionErrors {
			log.Warnf("OpenMetrics endpoint %s is not accessible", s.cfg.endpoint)
			return nil, nil
		}
		return nil, err
	}
	if response.StatusCode >= 400 && s.cfg.authToken != nil {
		response.Body.Close()
		s.cfg.authToken.reset()
		if err := s.cfg.authToken.apply(request); err != nil {
			s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
			return nil, err
		}
		response, err = s.httpClient.Do(request)
		if err != nil {
			s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
			if s.cfg.ignoreConnectionErrors {
				log.Warnf("OpenMetrics endpoint %s is not accessible", s.cfg.endpoint)
				return nil, nil
			}
			return nil, err
		}
	}
	if response.StatusCode >= 400 {
		response.Body.Close()
		err := fmt.Errorf("unexpected status code %d scraping %s", response.StatusCode, s.cfg.endpoint)
		s.submitHealth(sender, servicecheck.ServiceCheckCritical, err.Error())
		return nil, err
	}

	s.submitHealth(sender, servicecheck.ServiceCheckOK, "")
	return &scrapeResponse{
		body:          response.Body,
		contentType:   response.Header.Get("Content-Type"),
		contentLength: response.ContentLength,
	}, nil
}

func (s *openmetricsScraper) payloadSize(response *scrapeResponse, bytesRead int) float64 {
	if response.contentLength >= 0 {
		return float64(response.contentLength)
	}
	return float64(bytesRead)
}

func (s *openmetricsScraper) bearerToken() (string, error) {
	if s.cfg.bearerToken != "" {
		return s.cfg.bearerToken, nil
	}
	s.bearerTokenMu.Lock()
	defer s.bearerTokenMu.Unlock()
	if s.cachedBearerToken != "" && time.Since(s.bearerTokenRefreshed) <= s.cfg.bearerTokenRefreshInterval {
		return s.cachedBearerToken, nil
	}
	path := s.cfg.bearerTokenPath
	if path == "" {
		path = defaultBearerTokenPath
	}
	token, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s.cachedBearerToken = strings.TrimSpace(string(token))
	s.bearerTokenRefreshed = time.Now()
	return s.cachedBearerToken, nil
}

func (s *openmetricsScraper) applyRawMetricPrefix(metric parsedMetric) parsedMetric {
	if s.cfg.rawMetricPrefix == "" {
		return metric
	}
	metric.Name = strings.TrimPrefix(metric.Name, s.cfg.rawMetricPrefix)
	return metric
}

func (s *openmetricsScraper) isMetricExcluded(metric parsedMetric) bool {
	if _, ok := s.excludeMetrics[metric.Name]; ok {
		return true
	}
	return s.excludeMetricPatterns != nil && s.excludeMetricPatterns.MatchString(metric.Name)
}

func (s *openmetricsScraper) generateSampleData(metric parsedMetric, sender sender.Sender) []sampleDatum {
	out := make([]sampleDatum, 0, len(metric.Samples))
	for _, sample := range metric.Samples {
		if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
			continue
		}

		labels := sample.Labels
		s.labelAggregator.populate(labels)
		normalizeSampleLabels(metric.Type, labels)

		tags := make([]string, 0, len(labels)+len(s.tags))
		skipSample := false
		for labelName, labelValue := range labels {
			if labelName == nameLabel {
				continue
			}
			if excluder := s.labelExcluders[labelName]; excluder != nil && excluder(labelValue) {
				skipSample = true
				break
			}
			if _, ok := s.excludeLabels[labelName]; ok {
				continue
			}
			if len(s.includeLabels) > 0 {
				if _, ok := s.includeLabels[labelName]; !ok {
					continue
				}
			}

			tagName := labelName
			if renamed := s.cfg.renameLabels[labelName]; renamed != "" {
				tagName = renamed
			}
			tags = append(tags, tagName+":"+labelValue)
		}
		if skipSample {
			continue
		}
		sort.Strings(tags)
		tags = append(tags, s.tags...)

		hostname := ""
		if s.cfg.hostnameLabel != "" {
			if labelValue, ok := labels[s.cfg.hostnameLabel]; ok {
				if labelValue != "" {
					hostname = labelValue
					if s.cfg.hostnameFormat != "" {
						hostname = strings.Replace(s.cfg.hostnameFormat, "<HOSTNAME>", hostname, 1)
					}
				}
			}
		}
		if s.cfg.telemetry {
			s.submitTelemetryCount(sender, "telemetry.metrics.processed.count", 1)
		}

		sample.Labels = labels
		out = append(out, sampleDatum{sample: sample, tags: tags, hostname: hostname})
	}
	return out
}

func (s *openmetricsScraper) submitHealth(sender sender.Sender, status servicecheck.ServiceCheckStatus, message string) {
	if !s.cfg.enableHealthServiceCheck {
		return
	}
	sender.ServiceCheck(namespacedMetric(s.cfg.namespace, s.cfg.healthServiceCheckName), status, "", s.staticTags, message)
}

func (s *openmetricsScraper) submitTelemetryCount(sender sender.Sender, metricName string, value float64) {
	if value == 0 {
		return
	}
	sender.Count(namespacedMetric(s.cfg.namespace, metricName), value, "", s.tags)
}

func (s *openmetricsScraper) submitTelemetryGauge(sender sender.Sender, metricName string, value float64) {
	sender.Gauge(namespacedMetric(s.cfg.namespace, metricName), value, "", s.tags)
}

func namespacedMetric(namespace, metricName string) string {
	if namespace == "" || metricName == "" {
		return metricName
	}
	return strings.TrimSuffix(namespace, ".") + "." + strings.TrimPrefix(metricName, ".")
}

func responseUsesOpenMetrics(contentType string) bool {
	mediaType := strings.Split(contentType, ";")[0]
	return strings.TrimSpace(strings.ToLower(mediaType)) == "application/openmetrics-text"
}

func staticTags(cfg *scraperConfig) ([]string, error) {
	ignoredTags, err := compileRegexList(cfg.ignoreTags)
	if err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(cfg.tags)+1)
	for _, tag := range cfg.tags {
		if ignoredTags != nil && ignoredTags.MatchString(tag) {
			continue
		}
		tags = append(tags, tag)
	}
	if cfg.tagByEndpoint {
		tags = append(tags, "endpoint:"+cfg.endpoint)
	}
	return tags, nil
}

func compileExcludeMetrics(entries []string) (map[string]struct{}, *regexp.Regexp, error) {
	exact := map[string]struct{}{}
	patterns := []string{}
	for _, entry := range entries {
		if entry == regexp.QuoteMeta(entry) {
			exact[entry] = struct{}{}
			continue
		}
		patterns = append(patterns, entry)
	}
	pattern, err := compileRegexList(patterns)
	if err != nil {
		return nil, nil, err
	}
	return exact, pattern, nil
}

func compileLabelExcluders(entries map[string]interface{}) (map[string]labelExcluder, error) {
	out := make(map[string]labelExcluder, len(entries))
	for label, rawValues := range entries {
		if value, ok := rawValues.(bool); ok {
			if value {
				out[label] = func(string) bool { return true }
				continue
			}
			return nil, fmt.Errorf("label `%s` of setting `exclude_metrics_by_labels` must be an array or set to `true`", label)
		}
		values := interfaceSliceToStrings(rawValues)
		if values == nil {
			return nil, fmt.Errorf("label `%s` of setting `exclude_metrics_by_labels` must be an array or set to `true`", label)
		}
		pattern, err := compileRegexList(values)
		if err != nil {
			return nil, err
		}
		compiledPattern := pattern
		out[label] = func(labelValue string) bool {
			return compiledPattern != nil && compiledPattern.MatchString(labelValue)
		}
	}
	return out, nil
}

func newHTTPClient(cfg *scraperConfig) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = !cfg.persistConnections
	dialer := &net.Dialer{Timeout: cfg.timeout}
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &readTimeoutConn{Conn: conn, timeout: cfg.timeout}, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: !cfg.tlsVerify || !cfg.tlsValidateHostname, //nolint:gosec // Chain verification is restored below when only hostname validation is disabled.
	}
	if cfg.tlsCACert != "" {
		roots := x509.NewCertPool()
		if err := appendCACertificates(roots, expandUserPath(cfg.tlsCACert)); err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = roots
	}
	if cfg.tlsUseHostHeader {
		if serverName := tlsServerNameFromHostHeader(cfg.headers); serverName != "" {
			tlsConfig.ServerName = serverName
		}
	}
	if err := configureTLSProtocols(tlsConfig, cfg.tlsProtocolsAllowed); err != nil {
		return nil, err
	}
	cipherSuites, err := tlsCipherSuites(cfg.tlsCiphers)
	if err != nil {
		return nil, err
	}
	if len(cipherSuites) > 0 {
		tlsConfig.CipherSuites = cipherSuites
	}
	if cfg.tlsCert != "" || cfg.tlsPrivateKey != "" {
		privateKey := cfg.tlsPrivateKey
		if privateKey == "" {
			privateKey = cfg.tlsCert
		}
		cert, err := loadX509KeyPair(expandUserPath(cfg.tlsCert), expandUserPath(privateKey), cfg.tlsPrivateKeyPassword)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if cfg.tlsVerify && !cfg.tlsValidateHostname {
		tlsConfig.VerifyConnection = verifyCertificateChain(tlsConfig.RootCAs)
	}
	transport.TLSClientConfig = tlsConfig

	if cfg.skipProxy {
		transport.Proxy = nil
	} else {
		defaultProxy := transport.Proxy
		if proxies := pkgconfigsetup.Datadog().GetProxies(); proxies != nil {
			defaultProxy = httputils.GetProxyTransportFunc(proxies, pkgconfigsetup.Datadog())
		}
		transport.Proxy = func(request *http.Request) (*url.URL, error) {
			if shouldBypassProxy(request.URL, cfg.noProxy) {
				return nil, nil
			}
			if proxyURL := proxyURLForEndpoint(request.URL.String(), cfg.proxy); proxyURL != nil {
				return proxyURL, nil
			}
			if defaultProxy == nil {
				return nil, nil
			}
			return defaultProxy(request)
		}
	}

	client := &http.Client{Transport: transport}
	if cfg.persistConnections {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		client.Jar = jar
	}
	if !cfg.allowRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client, nil
}

type readTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *readTimeoutConn) Read(buffer []byte) (int, error) {
	if err := c.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(buffer)
}

func basicAuthorization(username string, password string, legacyEncoding bool) (string, error) {
	credentials := username + ":" + password
	if !legacyEncoding {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials)), nil
	}

	latin1 := make([]byte, 0, len(credentials))
	for _, char := range credentials {
		if char > 255 {
			return "", fmt.Errorf("basic auth credentials contain character %q that cannot be encoded as latin-1", char)
		}
		latin1 = append(latin1, byte(char))
	}
	return "Basic " + base64.StdEncoding.EncodeToString(latin1), nil
}

func expandUserPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + strings.TrimPrefix(path, "~")
		}
	}
	return path
}

func appendCACertificates(pool *x509.CertPool, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		pemData, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return fmt.Errorf("unable to load CA certificate from %s", path)
		}
		return nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	loaded := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		pemData, err := os.ReadFile(path + string(os.PathSeparator) + entry.Name())
		if err != nil {
			return err
		}
		loaded = pool.AppendCertsFromPEM(pemData) || loaded
	}
	if !loaded {
		return fmt.Errorf("unable to load CA certificates from %s", path)
	}
	return nil
}

func loadX509KeyPair(certPath string, keyPath string, password string) (tls.Certificate, error) {
	if password == "" {
		return tls.LoadX509KeyPair(certPath, keyPath)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	block, rest := pem.Decode(keyPEM)
	if block == nil {
		return tls.Certificate{}, fmt.Errorf("unable to decode private key from %s", keyPath)
	}
	if !x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck // Required for compatibility with encrypted PEM keys accepted by the Python check.
		return tls.Certificate{}, unsupportedCoreConfig("tls_private_key_password with a non-legacy encrypted key")
	}
	decrypted, err := x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck // See compatibility note above.
	if err != nil {
		return tls.Certificate{}, err
	}
	decryptedPEM := append(pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: decrypted}), rest...)
	return tls.X509KeyPair(certPEM, decryptedPEM)
}

func verifyCertificateChain(roots *x509.CertPool) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("server did not provide a TLS certificate")
		}
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates})
		return err
	}
}

func tlsServerNameFromHostHeader(headers map[string]string) string {
	for header, value := range headers {
		if !strings.EqualFold(header, "Host") {
			continue
		}
		if host, _, err := net.SplitHostPort(value); err == nil {
			return host
		}
		return value
	}
	return ""
}

func configureTLSProtocols(config *tls.Config, protocols []string) error {
	if len(protocols) == 0 {
		return nil
	}
	allowedVersions := map[uint16]struct{}{}
	for _, protocol := range protocols {
		version, ok := tlsProtocolVersion(protocol)
		if !ok {
			log.Warnf("Unknown TLS protocol `%s` configured, ignoring it.", protocol)
			continue
		}
		if version == 0 {
			log.Warnf("TLS protocol `%s` is not supported by Go, ignoring it.", protocol)
			continue
		}
		allowedVersions[version] = struct{}{}
		if config.MinVersion == 0 || version < config.MinVersion {
			config.MinVersion = version
		}
		if version > config.MaxVersion {
			config.MaxVersion = version
		}
	}
	if len(allowedVersions) == 0 {
		return nil
	}
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if _, ok := allowedVersions[state.Version]; ok {
			return nil
		}
		return fmt.Errorf("protocol version `%s` not in the allowed list %v", tlsProtocolName(state.Version), protocols)
	}
	return nil
}

func tlsProtocolVersion(protocol string) (uint16, bool) {
	switch protocol {
	case "SSLv3":
		return 0, true
	case "TLSv1":
		return tls.VersionTLS10, true
	case "TLSv1.1":
		return tls.VersionTLS11, true
	case "TLSv1.2":
		return tls.VersionTLS12, true
	case "TLSv1.3":
		return tls.VersionTLS13, true
	default:
		return 0, false
	}
}

func tlsProtocolName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLSv1"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS13:
		return "TLSv1.3"
	default:
		return fmt.Sprintf("0x%x", version)
	}
}

func proxyURLForEndpoint(endpoint string, proxies map[string]string) *url.URL {
	if len(proxies) == 0 {
		return nil
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil
	}
	rawProxy := proxies[parsedEndpoint.Scheme]
	if rawProxy == "" {
		rawProxy = proxies["url"]
	}
	if rawProxy == "" {
		return nil
	}
	parsedProxy, err := url.Parse(rawProxy)
	if err != nil {
		log.Warnf("Ignoring invalid proxy URL for OpenMetrics endpoint %s: %s", endpoint, err)
		return nil
	}
	return parsedProxy
}

func shouldBypassProxy(endpoint *url.URL, noProxy []string) bool {
	if len(noProxy) == 0 {
		return false
	}
	if endpoint.Scheme == "unix" {
		return true
	}
	host := strings.ToLower(endpoint.Hostname())
	if host == "" {
		return false
	}
	hostIP := net.ParseIP(host)
	for _, entry := range noProxy {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == "*" {
			return true
		}
		if proxyNetwork, ok := parseNoProxyNetwork(entry); ok {
			if hostIP != nil && proxyNetwork.Contains(hostIP) {
				return true
			}
			continue
		}
		if strings.HasPrefix(entry, ".") || strings.HasPrefix(entry, "*.") {
			if strings.HasSuffix(host, strings.TrimPrefix(entry, "*")) {
				return true
			}
			continue
		}
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

func parseNoProxyNetwork(entry string) (*net.IPNet, bool) {
	if !strings.Contains(entry, "/") {
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, false
		}
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, true
	}
	if _, network, err := net.ParseCIDR(entry); err == nil {
		return network, true
	}
	parts := strings.Split(entry, "/")
	if len(parts) != 2 {
		return nil, false
	}
	ip := net.ParseIP(parts[0])
	maskIP := net.ParseIP(parts[1])
	if ip == nil || maskIP == nil {
		return nil, false
	}
	ip4 := ip.To4()
	mask4 := maskIP.To4()
	if ip4 == nil || mask4 == nil {
		return nil, false
	}
	mask := net.IPv4Mask(mask4[0], mask4[1], mask4[2], mask4[3])
	ones, bits := mask.Size()
	if ones == 0 && bits == 0 {
		inverted := net.IPv4Mask(^mask4[0], ^mask4[1], ^mask4[2], ^mask4[3])
		if invertedOnes, invertedBits := inverted.Size(); invertedBits != 0 {
			mask = inverted
			ones = invertedOnes
			bits = invertedBits
		}
	}
	if bits == 0 {
		return nil, false
	}
	return &net.IPNet{IP: ip4.Mask(mask), Mask: net.CIDRMask(ones, bits)}, true
}

func tlsCipherSuites(configured []string) ([]uint16, error) {
	if len(configured) == 0 {
		return nil, nil
	}
	ciphers := make([]uint16, 0, len(configured))
	for _, cipher := range configured {
		cipher = strings.TrimSpace(cipher)
		if cipher == "" || strings.EqualFold(cipher, "ALL") {
			return nil, nil
		}
		cipherID, ok := tlsCipherSuite(cipher)
		if !ok {
			return nil, fmt.Errorf("unsupported TLS cipher `%s`", cipher)
		}
		if cipherID != 0 {
			ciphers = append(ciphers, cipherID)
		}
	}
	return ciphers, nil
}

func tlsCipherSuite(cipher string) (uint16, bool) {
	switch strings.ToUpper(cipher) {
	case "TLS_RSA_WITH_AES_128_CBC_SHA", "AES128-SHA":
		return tls.TLS_RSA_WITH_AES_128_CBC_SHA, true
	case "TLS_RSA_WITH_AES_256_CBC_SHA", "AES256-SHA":
		return tls.TLS_RSA_WITH_AES_256_CBC_SHA, true
	case "TLS_RSA_WITH_AES_128_GCM_SHA256", "AES128-GCM-SHA256":
		return tls.TLS_RSA_WITH_AES_128_GCM_SHA256, true
	case "TLS_RSA_WITH_AES_256_GCM_SHA384", "AES256-GCM-SHA384":
		return tls.TLS_RSA_WITH_AES_256_GCM_SHA384, true
	case "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", "ECDHE-RSA-AES128-SHA":
		return tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, true
	case "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA", "ECDHE-RSA-AES256-SHA":
		return tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, true
	case "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", "ECDHE-RSA-AES128-GCM-SHA256":
		return tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, true
	case "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", "ECDHE-RSA-AES256-GCM-SHA384":
		return tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, true
	case "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA", "ECDHE-ECDSA-AES128-SHA":
		return tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, true
	case "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA", "ECDHE-ECDSA-AES256-SHA":
		return tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, true
	case "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", "ECDHE-ECDSA-AES128-GCM-SHA256":
		return tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, true
	case "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", "ECDHE-ECDSA-AES256-GCM-SHA384":
		return tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, true
	case "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", "ECDHE-RSA-CHACHA20-POLY1305":
		return tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, true
	case "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256", "ECDHE-ECDSA-CHACHA20-POLY1305":
		return tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, true
	case "TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384", "TLS_CHACHA20_POLY1305_SHA256":
		return 0, true
	default:
		return 0, false
	}
}
