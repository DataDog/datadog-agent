// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package connectivity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	logstcp "github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const requestWithHeader = "datadog-agent-diagnose"

func getLogsEndpoints(useTCP bool) (*logsConfig.Endpoints, error) {
	datadogConfig := pkgconfigsetup.Datadog()
	logsConfigKey := logsConfig.NewLogsConfigKeys("logs_config.", datadogConfig)

	protocol := logsConfig.DiagnosticHTTP
	if useTCP {
		protocol = logsConfig.DiagnosticTCP
	}

	return logsConfig.BuildEndpointsForDiagnostic(
		datadogConfig,
		logsConfigKey,
		logsConfig.DefaultDiagnosticPrefix,
		protocol,
		"logs",
		logsConfig.AgentJSONIntakeProtocol,
		logsConfig.DefaultIntakeOrigin,
	)
}

// getLogsUseTCP returns true if the agent should use TCP to transport logs
func getLogsUseTCP() bool {
	datadogConfig := pkgconfigsetup.Datadog()
	useTCP := datadogConfig.GetBool("logs_config.force_use_tcp") && !datadogConfig.GetBool("logs_config.force_use_http")

	return useTCP
}

type connDiagnostician struct {
	diagCfg         diagnose.Config
	log             log.Component
	domainResolvers map[string]resolver.DomainResolver
	client          *http.Client
}

func newConnectivityDiagnostician(diagCfg diagnose.Config, log log.Component) *connDiagnostician {
	return &connDiagnostician{
		diagCfg: diagCfg,
		log:     log,
	}
}

func (cd *connDiagnostician) initDomainResolvers() diagnose.Diagnosis {
	config := pkgconfigsetup.Datadog()
	cd.client = getClient(config, 1, cd.log)

	// Create standard domain resolvers
	endpointDescriptors, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisSuccess,
			Name:        "Endpoints configuration",
			Diagnosis:   "Misconfiguration of agent endpoints",
			Remediation: "Please validate Agent configuration",
			RawError:    err.Error(),
		}
	}
	cd.domainResolvers, err = resolver.NewSingleDomainResolvers2(endpointDescriptors)
	if err != nil {
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisSuccess,
			Name:        "Resolver error",
			Diagnosis:   "Unable to create domain resolver",
			Remediation: "This is likely due to a bug",
			RawError:    err.Error(),
		}
	}

	return diagnose.Diagnosis{
		Status: diagnose.DiagnosisSuccess,
	}
}

func (cd *connDiagnostician) diagnose() []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis

	// Create diagnosis for logs
	if pkgconfigsetup.Datadog().GetBool("logs_enabled") {
		diag := cd.checkLogsEndpoint()
		diagnoses = append(diagnoses, diag)
	}

	endpointsInfo := getEndpointsInfo(pkgconfigsetup.Datadog())

	// Send requests to all endpoints for all domains
	for _, domainResolver := range cd.domainResolvers {
		// Go through all API Keys of a domain and send an HTTP request on each endpoint
		for _, apiKey := range domainResolver.GetAPIKeys() {
			for _, endpointInfo := range endpointsInfo {
				diag := cd.checkEndpoint(domainResolver, endpointInfo, apiKey)
				diagnoses = append(diagnoses, diag)
			}
		}
	}
	return diagnoses
}

func (cd *connDiagnostician) checkLogsEndpoint() diagnose.Diagnosis {
	useTCP := getLogsUseTCP()
	endpoints, err := getLogsEndpoints(useTCP)

	if err != nil {
		return diagnose.Diagnosis{
			Status:      diagnose.DiagnosisFail,
			Name:        "Endpoints configuration",
			Diagnosis:   "Misconfiguration of agent endpoints",
			Remediation: "Please validate agent configuration",
			RawError:    err.Error(),
		}
	}

	var url string
	connType := "HTTPS"
	if useTCP {
		connType = "TCP"
		url, err = logstcp.CheckConnectivityDiagnose(endpoints.Main, 5)
	} else {
		url, err = logshttp.CheckConnectivityDiagnose(endpoints.Main, pkgconfigsetup.Datadog())
	}

	name := fmt.Sprintf("%s connectivity to %s", connType, url)
	return createDiagnosis(name, url, "", err)
}

func (cd *connDiagnostician) checkEndpointURL(url string, endpointInfo endpointInfo, apiKey string) diagnose.Diagnosis {
	var responseBody []byte
	var err error
	var statusCode int
	var httpTraces []string

	if endpointInfo.Method == "HEAD" {
		statusCode, err = sendHTTPHEADRequestToEndpoint(url, getClient(pkgconfigsetup.Datadog(), 1, cd.log, withOneRedirect()))
	} else {
		httpTraces = []string{}
		ctx := httptrace.WithClientTrace(context.Background(), createDiagnoseTraces(&httpTraces, false))

		statusCode, responseBody, _, err = sendHTTPRequestToEndpoint(ctx, cd.client, url, endpointInfo, apiKey)
	}

	// Check if there is a response and if it's valid
	report, reportErr := verifyEndpointResponse(cd.diagCfg, statusCode, responseBody, err)

	diagnosisName := "Connectivity to " + url
	diag := createDiagnosis(diagnosisName, url, report, reportErr)

	// Prepend http trace on error or if in verbose mode
	if len(httpTraces) > 0 && (cd.diagCfg.Verbose || reportErr != nil) {
		diag.Diagnosis = fmt.Sprintf("\n%s\n%s", strings.Join(httpTraces, "\n"), diag.Diagnosis)
	}
	return diag
}

func (cd *connDiagnostician) checkEndpoint(domainResolver resolver.DomainResolver, endpointInfo endpointInfo, apiKey string) diagnose.Diagnosis {
	var url string

	if endpointInfo.Endpoint.Name == "flare" {
		url = endpointInfo.Endpoint.Route
	} else {
		domain := domainResolver.Resolve(endpointInfo.Endpoint)
		url = createEndpointURL(domain, endpointInfo)
	}
	diag := cd.checkEndpointURL(url, endpointInfo, apiKey)

	// Detect if the connection may have failed because a FQDN was used, by checking if one with a PQDN succeeds
	if diag.Status != diagnose.DiagnosisSuccess && pkgconfigsetup.Datadog().GetBool("convert_dd_site_fqdn.enabled") && URLhasFQDN(url) {
		pqdnURL, err := URLwithPQDN(url)
		if err != nil {
			cd.log.Errorf("can't convert URL to PQDN: %s", err)
			return diag
		}
		cd.log.Infof("The connection to %q with a FQDN failed; attempting to connect to %q", url, pqdnURL)

		d := cd.checkEndpointURL(pqdnURL, endpointInfo, apiKey)
		if d.Status == diagnose.DiagnosisSuccess {
			diag.Remediation = fmt.Sprintf(
				"The connection to the fully qualified domain name (FQDN) %q failed, but the connection to %q (without trailing dot) succeeded. Update your firewall and/or proxy configuration to accept FQDN connections, or disable FQDN usage by setting `convert_dd_site_fqdn.enabled` to false in the agent configuration.",
				url, pqdnURL)
		}
	}
	return diag
}

func URLhasFQDN(u string) bool {
	url, err := url.Parse(u)
	return err == nil && strings.HasSuffix(url.Hostname(), ".")
}

func URLwithPQDN(u string) (string, error) {
	url, err := url.Parse(u)
	if err != nil {
		return "", errors.New("Route is not a valid URL")
	}

	host := strings.TrimSuffix(url.Host, ".")
	if port := url.Port(); port != "" {
		url.Host = fmt.Sprintf("%s:%s", host, port)
	} else {
		url.Host = host
	}
	return url.String(), nil
}

// Diagnose performs connectivity diagnosis
func Diagnose(diagCfg diagnose.Config, log log.Component) []diagnose.Diagnosis {

	connDiagnostician := newConnectivityDiagnostician(diagCfg, log)
	diag := connDiagnostician.initDomainResolvers()
	if diag.Status != diagnose.DiagnosisSuccess {
		return []diagnose.Diagnosis{diag}
	}

	return connDiagnostician.diagnose()
}

func createDiagnosis(name string, logURL string, report string, err error) diagnose.Diagnosis {
	d := diagnose.Diagnosis{
		Name: name,
	}

	if err == nil {
		d.Status = diagnose.DiagnosisSuccess
		diagnosisWithoutReport := fmt.Sprintf("Connectivity to `%s` is Ok", logURL)
		d.Diagnosis = createDiagnosisString(diagnosisWithoutReport, report)
	} else {
		d.Status = diagnose.DiagnosisFail
		diagnosisWithoutReport := fmt.Sprintf("Connection to `%s` failed", logURL)
		d.Diagnosis = createDiagnosisString(diagnosisWithoutReport, report)
		d.Remediation = "Please validate Agent configuration and firewall to access " + logURL
		d.RawError = err.Error()
	}

	return d
}

func createDiagnosisString(diagnosis string, report string) string {
	if len(report) == 0 {
		return diagnosis
	}

	return fmt.Sprintf("%v\n%v", diagnosis, report)
}

// sendHTTPRequestToEndpoint creates an URL based on the domain and the endpoint information
// then sends an HTTP Request with the method and payload inside the endpoint information
func sendHTTPRequestToEndpoint(ctx context.Context, client *http.Client, url string, endpointInfo endpointInfo, apiKey string) (int, []byte, string, error) {
	headers := map[string]string{
		"Content-Type":     endpointInfo.ContentType,
		"DD-API-KEY":       apiKey,
		"DD-Agent-Version": version.AgentVersion,
		"User-Agent":       "datadog-agent/" + version.AgentVersion,
		"X-Requested-With": requestWithHeader,
	}

	return sendRequest(ctx, client, url, endpointInfo.Method, endpointInfo.Payload, headers)
}

// createEndpointUrl joins a domain with an endpoint
func createEndpointURL(domain string, endpointInfo endpointInfo) string {
	return domain + endpointInfo.Endpoint.Route
}

// vertifyEndpointResponse interprets the endpoint response and displays information on if the connectivity
// check was successful or not
func verifyEndpointResponse(diagCfg diagnose.Config, statusCode int, responseBody []byte, err error) (string, error) {
	if err != nil {
		return fmt.Sprintf("Could not get a response from the endpoint : %v\n%s\n",
			scrubber.ScrubLine(err.Error()), noResponseHints(err)), err
	}

	var verifyReport string
	var newErr error

	limitSize := 500

	scrubbedResponseBody := scrubber.ScrubLine(string(responseBody))
	if !diagCfg.Verbose && len(scrubbedResponseBody) > limitSize {
		scrubbedResponseBody = scrubbedResponseBody[:limitSize] + "...\n"
		scrubbedResponseBody += fmt.Sprintf("Response body is %v bytes long, truncated at %v\n", len(responseBody), limitSize)
		scrubbedResponseBody += "To display the whole body use the \"--verbose\" flag."
	}

	if statusCode >= 400 {
		newErr = errors.New("bad request")
		verifyReport = fmt.Sprintf("Received response : '%v'\n", scrubbedResponseBody)
	}

	verifyReport += fmt.Sprintf("Received status code %v from the endpoint", statusCode)
	return verifyReport, newErr
}

// noResponseHints aims to give hints when the endpoint did not respond.
// For instance, when sending an HTTP request to a HAProxy endpoint configured for HTTPS
// the endpoint send an empty response. As the error 'EOF' is not very informative, it can
// be interesting to 'wrap' this error to display more context.
func noResponseHints(err error) string {
	endpoint := utils.GetInfraEndpoint(pkgconfigsetup.Datadog())
	parsedURL, parseErr := url.Parse(endpoint)
	if parseErr != nil {
		return fmt.Sprintf("Could not parse url '%v' : %v", scrubber.ScrubLine(endpoint), scrubber.ScrubLine(parseErr.Error()))
	}

	if parsedURL.Scheme == "http" {
		if strings.Contains(err.Error(), "EOF") {
			return fmt.Sprintf("Hint: received an empty reply from the server. You are maybe trying to contact an HTTPS endpoint using an HTTP url: '%v'\n",
				scrubber.ScrubLine(endpoint))
		}
	}

	return ""
}

// See if the URL is redirected to another URL, and return the status code of the redirection
func sendHTTPHEADRequestToEndpoint(url string, client *http.Client) (int, error) {
	statusCode, _, err := sendHead(context.Background(), client, url)
	if err != nil {
		return -1, err
	}
	// Expected status codes are OK or a redirection
	if statusCode == http.StatusTemporaryRedirect || statusCode == http.StatusPermanentRedirect || statusCode == http.StatusOK {
		return statusCode, nil
	}
	return statusCode, fmt.Errorf("The request wasn't redirected nor achieving his goal: %v", statusCode)
}
