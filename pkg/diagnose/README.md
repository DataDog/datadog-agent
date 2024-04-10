# package `diagnose`

This package is used to register and run agent diagnostics which validates various aspects of agent installation, configuration and run-time environment.

## Running ```agent diagnose all```
Details and command line options are specified in [```agent diagnose```](/cmd/agent/subcommands/diagnose/README.md) command
You can run all registered diagnosis with the `diagnose` command on the agent

The `flare` command will also run registered diagnosis and output them in a `diagnose.log` file.

## Registering a new diagnose suite
All function and structures details can be found in [loader.go](./diagnosis/loader.go) file.

To register a diagnose suite one need to register a function which returns ```[]diagnosis.Diagnosis```.

Example from [loader.go](./connectivity/core_endpoint.go) file:
```
package connectivity

import (
    ...
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
    ...
)
...
func init() {
	diagnosis.Register("connectivity-datadog-core-endpoints", diagnose)
}
...
func diagnose(diagCfg diagnosis.Config) []diagnosis.Diagnosis {
    ...
    var diagnoses []diagnosis.Diagnosis
    ...
	for _, domainResolver := range domainResolvers {
        ...
		for _, apiKey := range domainResolver.GetAPIKeys() {
			for _, endpointInfo := range endpointsInfo {
                ...
				name := "Connectivity to " + logURL
				if reportErr == nil {
					diagnoses = append(diagnoses, diagnosis.Diagnosis{
						Result:    diagnosis.DiagnosisSuccess,
						Name:      name,
						Diagnosis: fmt.Sprintf("Connectivity to `%s` is Ok\n%s", logURL, report),
					})
				} else {
					diagnoses = append(diagnoses, diagnosis.Diagnosis{
						Result:      diagnosis.DiagnosisFail,
						Name:        name,
						Diagnosis:   fmt.Sprintf("Connection to `%s` failed\n%s", logURL, report),
						Remediation: "Please validate Agent configuration and firewall to access " + logURL,
						RawError:    err,
					})
				}
			}
		}
	}

	return diagnoses
}
```

## Context of a diagnose function execution
Normally, registered diagnose suite functions will be executed in context of the running agent service (or other services) but if ```Config.ForceLocal``` configuration is specified the registered diagnose function will be executed in the context of agent diagnose CLI command (if possible).

## Which connectivity to endpoint are tested ?
With diagnose command, the Agent try to reach out a lot of endpoints, these ones are listed below:

| Subdomain | Route | HTTP Method | Status Code expected |
|-----------|-------|-------------|----------------------|
| https://app.datadoghq.com | /api/v1/series | POST | 200 |
| https://app.datadoghq.com | /api/v1/check_run | POST | 200 |
| https://app.datadoghq.com | /intake/ | POST | 200 |
| https://app.datadoghq.com | /api/v1/validate | GET | 200 |
| https://app.datadoghq.com | /api/v1/metadata | POST | 200 |
| https://app.datadoghq.com | /api/v2/series | POST | 200 |
| https://app.datadoghq.com | /api/beta/sketches | POST | 200 |
| https://\<Agent_version>-flare.agent.datadoghq.com | /support/flare | HEAD | 307 |
| https://process.datadoghq.com | /intake/status | GET | 200 |



