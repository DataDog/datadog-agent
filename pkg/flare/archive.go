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

	"gopkg.in/yaml.v2"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare/common"
	"github.com/DataDog/datadog-agent/pkg/flare/priviledged"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	systemprobeStatus "github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// RemoteFlareProvider is a struct that contains a SecureClient
// It is used to make secure IPC requests to the agent
type RemoteFlareProvider struct {
	IPC ipc.Component
}

// getProcessAPIAddress is an Alias to GetProcessAPIAddressPort using Datadog config
func getProcessAPIAddressPort() (string, error) {
	return pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
}

// ExtraFlareProviders returns flare providers that are not given via fx.
// This function should only be called by the flare component.
func ExtraFlareProviders(workloadmeta option.Option[workloadmeta.Component], ipc ipc.Component) []*flaretypes.FlareFiller {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze what is being added and ensure that it contains no credentials
	 * or unnecessary user-specific data. The FlareBuilder scrubs secrets that match pre-programmed patterns, but it
	 * is always better to not capture data containing secrets, than to scrub that data.
	 */

	remote := &RemoteFlareProvider{
		IPC: ipc,
	}

	providers := []*flaretypes.FlareFiller{
		flaretypes.NewFiller(remote.provideExtraFiles),
		flaretypes.NewFiller(provideSystemProbe),
		flaretypes.NewFiller(remote.provideConfigDump),
		flaretypes.NewFiller(remote.provideRemoteConfig),
		flaretypes.NewFiller(getRegistryJSON),
		flaretypes.NewFiller(getVersionHistory),
		flaretypes.NewFiller(getWindowsData),
		flaretypes.NewFiller(common.GetExpVar),
		flaretypes.NewFiller(provideInstallInfo),
		flaretypes.NewFiller(provideAuthTokenPerm),
		flaretypes.NewFiller(provideContainers(workloadmeta)),
	}

	pprofURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		pkgconfigsetup.Datadog().GetString("expvar_port"))
	telemetryURL := fmt.Sprintf("http://127.0.0.1:%s/telemetry", pkgconfigsetup.Datadog().GetString("expvar_port"))

	for filename, fromFunc := range map[string]func() ([]byte, error){
		"envvars.log":         common.GetEnvVars,
		"health.yaml":         getHealth,
		"go-routine-dump.log": func() ([]byte, error) { return remote.getHTTPCallContent(pprofURL) },
		"telemetry.log":       func() ([]byte, error) { return remote.getHTTPCallContent(telemetryURL) },
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

func provideContainers(workloadmeta option.Option[workloadmeta.Component]) func(fb flaretypes.FlareBuilder) error {
	return func(fb flaretypes.FlareBuilder) error {
		fb.AddFileFromFunc("docker_ps.log", getDockerPs)                                                               //nolint:errcheck
		fb.AddFileFromFunc("k8s/kubelet_config.yaml", getKubeletConfig)                                                //nolint:errcheck
		fb.AddFileFromFunc("k8s/kubelet_pods.yaml", getKubeletPods)                                                    //nolint:errcheck
		fb.AddFileFromFunc("ecs_metadata.json", getECSMeta)                                                            //nolint:errcheck
		fb.AddFileFromFunc("docker_inspect.log", func() ([]byte, error) { return getDockerSelfInspect(workloadmeta) }) //nolint:errcheck

		return nil
	}
}

func provideAuthTokenPerm(fb flaretypes.FlareBuilder) error {
	fb.RegisterFilePerm(security.GetAuthTokenFilepath(pkgconfigsetup.Datadog()))
	return nil
}

func provideInstallInfo(fb flaretypes.FlareBuilder) error {
	fb.CopyFileTo(installinfo.GetFilePath(pkgconfigsetup.Datadog()), "install_info.log") //nolint:errcheck
	return nil
}

func (r *RemoteFlareProvider) provideRemoteConfig(fb flaretypes.FlareBuilder) error {
	if pkgconfigsetup.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		if err := r.exportRemoteConfig(fb); err != nil {
			log.Errorf("Could not export remote-config state: %s", err)
		}
	}
	return nil
}

func (r *RemoteFlareProvider) provideConfigDump(fb flaretypes.FlareBuilder) error {
	fb.AddFileFromFunc("process_agent_runtime_config_dump.yaml", r.getProcessAgentFullConfig)                                              //nolint:errcheck
	fb.AddFileFromFunc("runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(pkgconfigsetup.Datadog().AllSettings()) }) //nolint:errcheck
	return nil
}

func getVPCSubnetsForHost() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subnets, err := network.GetVPCSubnetsForHost(ctx)
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

func (r *RemoteFlareProvider) provideExtraFiles(fb flaretypes.FlareBuilder) error {
	if fb.IsLocal() {
		// Can't reach the agent, mention it in those two files
		fb.AddFile("status.log", []byte("unable to get the status of the agent, is it running?"))           //nolint:errcheck
		fb.AddFile("config-check.log", []byte("unable to get loaded checks config, is the agent running?")) //nolint:errcheck
	} else {
		fb.AddFileFromFunc("tagger-list.json", r.getAgentTaggerList)    //nolint:errcheck
		fb.AddFileFromFunc("workload-list.log", r.getAgentWorkloadList) //nolint:errcheck
		if !pkgconfigsetup.Datadog().GetBool("process_config.run_in_core_agent.enabled") {
			fb.AddFileFromFunc("process-agent_tagger-list.json", r.getProcessAgentTaggerList) //nolint:errcheck
			r.getChecksFromProcessAgent(fb, getProcessAPIAddressPort)
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
func (r *RemoteFlareProvider) getProcessAgentFullConfig() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	procStatusURL := fmt.Sprintf("https://%s/config/all", addressPort)

	bytes, err := r.IPC.GetClient().Get(procStatusURL, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		return []byte("error: process-agent is not running or is unreachable\n"), nil
	}
	return bytes, nil
}

func (r *RemoteFlareProvider) getChecksFromProcessAgent(fb flaretypes.FlareBuilder, getAddressPort func() (url string, err error)) {
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

		err := fb.AddFileFromFunc(filename, func() ([]byte, error) {
			return r.IPC.GetClient().Get(checkURL+checkName, ipchttp.WithLeaveConnectionOpen)
		})
		if err != nil {
			fb.AddFile( //nolint:errcheck
				filename,
				[]byte(err.Error()),
			)
		}
	}

	getCheck("process", "process_config.process_collection.enabled")
	getCheck("container", "process_config.container_collection.enabled")
	getCheck("process_discovery", "process_config.process_discovery.enabled")
}

func (r *RemoteFlareProvider) getAgentTaggerList() ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))

	return r.GetTaggerList(taggerListURL)
}

func (r *RemoteFlareProvider) getProcessAgentTaggerList() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	taggerListURL := fmt.Sprintf("https://%s/agent/tagger-list", addressPort)
	return r.GetTaggerList(taggerListURL)
}

// GetTaggerList fetches the tagger list from the given URL.
func (r *RemoteFlareProvider) GetTaggerList(remoteURL string) ([]byte, error) {
	resp, err := r.IPC.GetClient().Get(remoteURL, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		return nil, err
	}

	// Pretty print JSON output
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	err = json.Indent(&b, resp, "", "\t")
	if err != nil {
		return resp, nil
	}
	writer.Flush()

	return b.Bytes(), nil
}

func (r *RemoteFlareProvider) getAgentWorkloadList() ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	return r.GetWorkloadList(fmt.Sprintf("https://%v:%v/agent/workload-list?verbose=true", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port")))
}

// GetWorkloadList fetches the workload list from the given URL.
func (r *RemoteFlareProvider) GetWorkloadList(url string) ([]byte, error) {
	resp, err := r.IPC.GetClient().Get(url, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		return nil, err
	}

	workload := workloadmeta.WorkloadDumpResponse{}
	err = json.Unmarshal(resp, &workload)
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
func (r *RemoteFlareProvider) getHTTPCallContent(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.IPC.GetClient().Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return resp, nil
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
