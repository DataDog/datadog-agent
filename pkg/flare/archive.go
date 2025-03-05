// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare contains the logic to create a flare archive.
package flare

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/fatih/color"
	"gopkg.in/yaml.v2"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/flare/common"
	"github.com/DataDog/datadog-agent/pkg/flare/priviledged"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	systemprobeStatus "github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getProcessAPIAddress is an Alias to GetProcessAPIAddressPort using Datadog config
func getProcessAPIAddressPort() (string, error) {
	return pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
}

// ExtraFlareProviders returns flare providers that are not given via fx.
// This function should only be called by the flare component.
func ExtraFlareProviders(diagnoseDeps diagnose.SuitesDeps) []*flaretypes.FlareFiller {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze what is being added and ensure that it contains no credentials
	 * or unnecessary user-specific data. The FlareBuilder scrubs secrets that match pre-programmed patterns, but it
	 * is always better to not capture data containing secrets, than to scrub that data.
	 */

	providers := []*flaretypes.FlareFiller{
		flaretypes.NewFiller(provideExtraFiles),
		flaretypes.NewFiller(provideSystemProbe),
		flaretypes.NewFiller(provideConfigDump),
		flaretypes.NewFiller(provideRemoteConfig),
		flaretypes.NewFiller(getRegistryJSON),
		flaretypes.NewFiller(getVersionHistory),
		flaretypes.NewFiller(getWindowsData),
		flaretypes.NewFiller(common.GetExpVar),
		flaretypes.NewFiller(provideInstallInfo),
		flaretypes.NewFiller(provideAuthTokenPerm),
		flaretypes.NewFiller(provideDiagnoses(diagnoseDeps)),
		flaretypes.NewFiller(provideContainers(diagnoseDeps)),
	}

	pprofURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		pkgconfigsetup.Datadog().GetString("expvar_port"))
	telemetryURL := fmt.Sprintf("http://127.0.0.1:%s/telemetry", pkgconfigsetup.Datadog().GetString("expvar_port"))

	for filename, fromFunc := range map[string]func() ([]byte, error){
		"envvars.log":         common.GetEnvVars,
		"health.yaml":         getHealth,
		"go-routine-dump.log": func() ([]byte, error) { return getHTTPCallContent(pprofURL) },
		"telemetry.log":       func() ([]byte, error) { return getHTTPCallContent(telemetryURL) },
	} {
		providers = append(providers, flaretypes.NewFiller(
			func(fb flaretypes.FlareBuilder) error {
				fb.AddFileFromFunc(filename, fromFunc) //nolint:errcheck
				return nil
			},
		))
	}

	return providers
}

func provideContainers(diagnoseDeps diagnose.SuitesDeps) func(fb flaretypes.FlareBuilder) error {
	return func(fb flaretypes.FlareBuilder) error {
		fb.AddFileFromFunc("docker_ps.log", getDockerPs)                                                                          //nolint:errcheck
		fb.AddFileFromFunc("k8s/kubelet_config.yaml", getKubeletConfig)                                                           //nolint:errcheck
		fb.AddFileFromFunc("k8s/kubelet_pods.yaml", getKubeletPods)                                                               //nolint:errcheck
		fb.AddFileFromFunc("ecs_metadata.json", getECSMeta)                                                                       //nolint:errcheck
		fb.AddFileFromFunc("docker_inspect.log", func() ([]byte, error) { return getDockerSelfInspect(diagnoseDeps.GetWMeta()) }) //nolint:errcheck

		return nil
	}
}

func provideAuthTokenPerm(fb flaretypes.FlareBuilder) error {
	fb.RegisterFilePerm(security.GetAuthTokenFilepath(pkgconfigsetup.Datadog()))
	return nil
}

func provideDiagnoses(diagnoseDeps diagnose.SuitesDeps) func(fb flaretypes.FlareBuilder) error {
	return func(fb flaretypes.FlareBuilder) error {
		fb.AddFileFromFunc("diagnose.log", getDiagnoses(fb.IsLocal(), diagnoseDeps)) //nolint:errcheck
		return nil
	}
}

func provideInstallInfo(fb flaretypes.FlareBuilder) error {
	fb.CopyFile(installinfo.GetFilePath(pkgconfigsetup.Datadog())) //nolint:errcheck
	return nil
}

func provideRemoteConfig(fb flaretypes.FlareBuilder) error {
	if pkgconfigsetup.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		if err := exportRemoteConfig(fb); err != nil {
			log.Errorf("Could not export remote-config state: %s", err)
		}
	}
	return nil
}

func provideConfigDump(fb flaretypes.FlareBuilder) error {
	fb.AddFileFromFunc("process_agent_runtime_config_dump.yaml", getProcessAgentFullConfig)                                                //nolint:errcheck
	fb.AddFileFromFunc("runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(pkgconfigsetup.Datadog().AllSettings()) }) //nolint:errcheck
	return nil
}

func getVPCSubnetsForHost() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subnets, err := cloudproviders.GetVPCSubnetsForHost(ctx)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	for _, subnet := range subnets {
		buffer.WriteString(subnet.String() + "\n")
	}
	return buffer.Bytes(), nil
}

func provideSystemProbe(fb flaretypes.FlareBuilder) error {
	addSystemProbePlatformSpecificEntries(fb)

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		_ = fb.AddFileFromFunc(filepath.Join("expvar", "system-probe"), getSystemProbeStats)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "system_probe_telemetry.log"), getSystemProbeTelemetry)
		_ = fb.AddFileFromFunc("system_probe_runtime_config_dump.yaml", getSystemProbeConfig)
		_ = fb.AddFileFromFunc(filepath.Join("system-probe", "vpc_subnets.log"), getVPCSubnetsForHost)
	} else {
		// If system probe is disabled, we still want to include the system probe config file
		_ = fb.AddFileFromFunc("system_probe_runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(pkgconfigsetup.SystemProbe().AllSettings()) })
	}
	return nil
}

func provideExtraFiles(fb flaretypes.FlareBuilder) error {
	if fb.IsLocal() {
		// Can't reach the agent, mention it in those two files
		fb.AddFile("status.log", []byte("unable to get the status of the agent, is it running?"))           //nolint:errcheck
		fb.AddFile("config-check.log", []byte("unable to get loaded checks config, is the agent running?")) //nolint:errcheck
	} else {
		fb.AddFileFromFunc("tagger-list.json", getAgentTaggerList)    //nolint:errcheck
		fb.AddFileFromFunc("workload-list.log", getAgentWorkloadList) //nolint:errcheck
		if !pkgconfigsetup.Datadog().GetBool("process_config.run_in_core_agent.enabled") {
			fb.AddFileFromFunc("process-agent_tagger-list.json", getProcessAgentTaggerList) //nolint:errcheck
			getChecksFromProcessAgent(fb, getProcessAPIAddressPort)
		}
	}
	return nil
}

func getVersionHistory(fb flaretypes.FlareBuilder) error {
	fb.CopyFile(filepath.Join(pkgconfigsetup.Datadog().GetString("run_path"), "version-history.json")) //nolint:errcheck
	return nil
}

func getRegistryJSON(fb flaretypes.FlareBuilder) error {
	fb.CopyFile(filepath.Join(pkgconfigsetup.Datadog().GetString("logs_config.run_path"), "registry.json")) //nolint:errcheck
	return nil
}

func getSystemProbeStats() ([]byte, error) {
	// TODO: (components) - Temporary until we can use the status component to extract the system probe status from it.
	stats := map[string]interface{}{}
	systemprobeStatus.GetStatus(stats, priviledged.GetSystemProbeSocketPath())
	sysProbeBuf, err := yaml.Marshal(stats["systemProbeStats"])
	if err != nil {
		return nil, err
	}

	return sysProbeBuf, nil
}

func getSystemProbeTelemetry() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.URL("/telemetry")
	return getHTTPData(sysProbeClient, url)
}

func getSystemProbeConfig() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(priviledged.GetSystemProbeSocketPath())
	url := sysprobeclient.URL("/config")
	return getHTTPData(sysProbeClient, url)
}

// getProcessAgentFullConfig fetches process-agent runtime config as YAML and returns it to be added to  process_agent_runtime_config_dump.yaml
func getProcessAgentFullConfig() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	procStatusURL := fmt.Sprintf("https://%s/config/all", addressPort)

	bytes, err := getHTTPCallContent(procStatusURL)
	if err != nil {
		return []byte("error: process-agent is not running or is unreachable\n"), nil
	}
	return bytes, nil
}

func getChecksFromProcessAgent(fb flaretypes.FlareBuilder, getAddressPort func() (url string, err error)) {
	addressPort, err := getAddressPort()
	if err != nil {
		log.Errorf("Could not zip process agent checks: wrong configuration to connect to process-agent: %s", err.Error())
		return
	}
	checkURL := fmt.Sprintf("https://%s/check/", addressPort)

	getCheck := func(checkName, setting string) {
		filename := fmt.Sprintf("%s_check_output.json", checkName)

		if !pkgconfigsetup.Datadog().GetBool(setting) {
			fb.AddFile(filename, []byte(fmt.Sprintf("'%s' is disabled", setting))) //nolint:errcheck
			return
		}

		err := fb.AddFileFromFunc(filename, func() ([]byte, error) { return getHTTPCallContent(checkURL + checkName) })
		if err != nil {
			fb.AddFile( //nolint:errcheck
				"process_check_output.json",
				[]byte(fmt.Sprintf("error: process-agent is not running or is unreachable: %s", err.Error())),
			)
		}
	}

	getCheck("process", "process_config.process_collection.enabled")
	getCheck("container", "process_config.container_collection.enabled")
	getCheck("process_discovery", "process_config.process_discovery.enabled")
}

func getDiagnoses(isFlareLocal bool, deps diagnose.SuitesDeps) func() ([]byte, error) {
	fct := func(w io.Writer) error {
		// Run diagnose always "local" (in the host process that is)
		diagCfg := diagnosis.Config{
			Verbose:  true,
			RunLocal: true,
		}

		// ... but when running within Agent some diagnose suites need to know
		// that to run more optimally/differently by using existing in-memory objects
		collector, ok := deps.Collector.Get()
		if !isFlareLocal && ok {
			diagnoses, err := diagnose.RunInAgentProcess(diagCfg, diagnose.NewSuitesDepsInAgentProcess(collector))
			if err != nil {
				return err
			}
			return diagnose.RunDiagnoseStdOut(w, diagCfg, diagnoses)
		}

		diagnoseDeps := diagnose.NewSuitesDepsInCLIProcess(deps.SenderManager, deps.SecretResolver, deps.WMeta, deps.AC, deps.Tagger)
		diagnoses, err := diagnose.RunInCLIProcess(diagCfg, diagnoseDeps)
		if err != nil && !diagCfg.RunLocal {
			fmt.Fprintln(w, color.YellowString(fmt.Sprintf("Error running diagnose in Agent process: %s", err)))
			fmt.Fprintln(w, "Running diagnose command locally (may take extra time to run checks locally) ...")

			diagCfg.RunLocal = true
			diagnoses, err = diagnose.RunInCLIProcess(diagCfg, diagnoseDeps)
			if err != nil {
				fmt.Fprintln(w, color.RedString(fmt.Sprintf("Error running diagnose: %s", err)))
				return err
			}
		}
		return diagnose.RunDiagnoseStdOut(w, diagCfg, diagnoses)

	}

	return func() ([]byte, error) { return functionOutputToBytes(fct), nil }
}

func getAgentTaggerList() ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))

	return GetTaggerList(taggerListURL)
}

func getProcessAgentTaggerList() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	err = apiutil.SetAuthToken(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%s/agent/tagger-list", addressPort)
	return GetTaggerList(taggerListURL)
}

// GetTaggerList fetches the tagger list from the given URL.
func GetTaggerList(remoteURL string) ([]byte, error) {
	c := apiutil.GetClient(false) // FIX: get certificates right then make this true

	r, err := apiutil.DoGet(c, remoteURL, apiutil.LeaveConnectionOpen)
	if err != nil {
		return nil, err
	}

	// Pretty print JSON output
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	err = json.Indent(&b, r, "", "\t")
	if err != nil {
		return r, nil
	}
	writer.Flush()

	return b.Bytes(), nil
}

func getAgentWorkloadList() ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	return GetWorkloadList(fmt.Sprintf("https://%v:%v/agent/workload-list?verbose=true", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port")))
}

// GetWorkloadList fetches the workload list from the given URL.
func GetWorkloadList(url string) ([]byte, error) {
	c := apiutil.GetClient(false) // FIX: get certificates right then make this true

	r, err := apiutil.DoGet(c, url, apiutil.LeaveConnectionOpen)
	if err != nil {
		return nil, err
	}

	workload := workloadmeta.WorkloadDumpResponse{}
	err = json.Unmarshal(r, &workload)
	if err != nil {
		return nil, err
	}

	fct := func(w io.Writer) error {
		workload.Write(w)
		return nil
	}
	return functionOutputToBytes(fct), nil
}

func getHealth() ([]byte, error) {
	s := health.GetReady()
	sort.Strings(s.Healthy)
	sort.Strings(s.Unhealthy)

	yamlValue, err := yaml.Marshal(s)
	if err != nil {
		return nil, err
	}

	return yamlValue, nil
}

func getECSMeta() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	ecsMeta, err := ecs.NewECSMeta(ctx)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(ecsMeta, "", "\t")
}

// getHTTPCallContent does a GET HTTP call to the given url and
// writes the content of the HTTP response in the given file, ready
// to be shipped in a flare.
func getHTTPCallContent(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	client := apiutil.GetClient(false) // FIX: get certificates right then make this true

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// read the entire body, so that it can be scrubbed in its entirety
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// functionOutputToBytes runs a given function and returns its output in a byte array
// This is used when we want to capture the output of a function that normally prints on a terminal
func functionOutputToBytes(fct func(writer io.Writer) error) []byte {
	var buffer bytes.Buffer

	writer := bufio.NewWriter(&buffer)
	err := fct(writer)
	if err != nil {
		fmt.Fprintf(writer, "%s", err)
	}
	writer.Flush()

	return buffer.Bytes()
}

func getHTTPData(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-ok status code: url: %s, status_code: %d, response: `%s`", req.URL, resp.StatusCode, string(data))
	}
	return data, nil
}
