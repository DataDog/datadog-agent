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
	"expvar"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/flare/sysprobe"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	systemprobeStatus "github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
)

// Match .yaml and .yml to ship configuration files in the flare.
var cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

// searchPaths is a list of path where to look for checks configurations
type searchPaths map[string]string

// getProcessAPIAddress is an Alias to GetProcessAPIAddressPort using Datadog config
func getProcessAPIAddressPort() (string, error) {
	return pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
}

// ExtraFlareProviders returns flare providers that are not given via fx.
// This function should only be called by the flare component.
func ExtraFlareProviders(diagnoseDeps diagnose.SuitesDeps) []flaretypes.FlareCallback {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze what is being added and ensure that it contains no credentials
	 * or unnecessary user-specific data. The FlareBuilder scrubs secrets that match pre-programmed patterns, but it
	 * is always better to not capture data containing secrets, than to scrub that data.
	 */

	providers := []flaretypes.FlareCallback{
		provideExtraFiles,
		provideSystemProbe,
		provideConfigDump,
		provideRemoteConfig,
		getRegistryJSON,
		getVersionHistory,
		getWindowsData,
		getExpVar,
		provideInstallInfo,
		provideAuthTokenPerm,
		provideDiagnoses(diagnoseDeps),
		provideContainers(diagnoseDeps),
	}

	pprofURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		pkgconfigsetup.Datadog().GetString("expvar_port"))
	telemetryURL := fmt.Sprintf("http://127.0.0.1:%s/telemetry", pkgconfigsetup.Datadog().GetString("expvar_port"))

	for filename, fromFunc := range map[string]func() ([]byte, error){
		"envvars.log":         getEnvVars,
		"health.yaml":         getHealth,
		"go-routine-dump.log": func() ([]byte, error) { return getHTTPCallContent(pprofURL) },
		"telemetry.log":       func() ([]byte, error) { return getHTTPCallContent(telemetryURL) },
	} {
		providers = append(providers, func(fb flaretypes.FlareBuilder) error {
			fb.AddFileFromFunc(filename, fromFunc) //nolint:errcheck
			return nil
		})
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
	fb.AddFileFromFunc("process_agent_runtime_config_dump.yaml", getProcessAgentFullConfig)                                                                 //nolint:errcheck
	fb.AddFileFromFunc("runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(pkgconfigsetup.Datadog().AllSettings()) })                  //nolint:errcheck
	fb.AddFileFromFunc("system_probe_runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(pkgconfigsetup.SystemProbe().AllSettings()) }) //nolint:errcheck
	return nil
}

func provideSystemProbe(fb flaretypes.FlareBuilder) error {
	systemProbeConfigBPFDir := pkgconfigsetup.SystemProbe().GetString("system_probe_config.bpf_dir")
	if systemProbeConfigBPFDir != "" {
		fb.RegisterDirPerm(systemProbeConfigBPFDir)
	}
	addSystemProbePlatformSpecificEntries(fb)

	if pkgconfigsetup.SystemProbe().GetBool("system_probe_config.enabled") {
		fb.AddFileFromFunc(filepath.Join("expvar", "system-probe"), getSystemProbeStats)                         //nolint:errcheck
		fb.AddFileFromFunc(filepath.Join("system-probe", "system_probe_telemetry.log"), getSystemProbeTelemetry) // nolint:errcheck
		fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_cached.log"), getSystemProbeConntrackCached) // nolint:errcheck
		fb.AddFileFromFunc(filepath.Join("system-probe", "conntrack_host.log"), getSystemProbeConntrackHost)     // nolint:errcheck
		fb.AddFileFromFunc(filepath.Join("system-probe", "ebpf_btf_loader.log"), getSystemProbeBTFLoaderInfo)    // nolint:errcheck
	}
	return nil
}

func provideExtraFiles(fb flaretypes.FlareBuilder) error {
	if fb.IsLocal() {
		// Can't reach the agent, mention it in those two files
		fb.AddFile("status.log", []byte("unable to get the status of the agent, is it running?"))           //nolint:errcheck
		fb.AddFile("config-check.log", []byte("unable to get loaded checks config, is the agent running?")) //nolint:errcheck
	} else {
		fb.AddFileFromFunc("tagger-list.json", getAgentTaggerList)                      //nolint:errcheck
		fb.AddFileFromFunc("workload-list.log", getAgentWorkloadList)                   //nolint:errcheck
		fb.AddFileFromFunc("process-agent_tagger-list.json", getProcessAgentTaggerList) //nolint:errcheck
		if !pkgconfigsetup.Datadog().GetBool("process_config.run_in_core_agent.enabled") {
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

func getLogFiles(fb flaretypes.FlareBuilder, logFileDir string) {
	log.Flush()

	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFileDir), "logs", func(path string) bool { //nolint:errcheck
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	})
}

func getExpVar(fb flaretypes.FlareBuilder) error {
	variables := make(map[string]interface{})
	expvar.Do(func(kv expvar.KeyValue) {
		variable := make(map[string]interface{})
		json.Unmarshal([]byte(kv.Value.String()), &variable) //nolint:errcheck
		variables[kv.Key] = variable
	})

	// The callback above cannot return an error.
	// In order to properly ensure error checking,
	// it needs to be done in its own loop
	for key, value := range variables {
		yamlValue, err := yaml.Marshal(value)
		if err != nil {
			return err
		}

		err = fb.AddFile(filepath.Join("expvar", key), yamlValue)
		if err != nil {
			return err
		}
	}

	apmDebugPort := pkgconfigsetup.Datadog().GetInt("apm_config.debug.port")
	f := filepath.Join("expvar", "trace-agent")
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/vars", apmDebugPort))
	if err != nil {
		return fb.AddFile(f, []byte(fmt.Sprintf("Error retrieving vars: %v", err)))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fb.AddFile(f, []byte(fmt.Sprintf("Got response %s from /debug/vars:\n%s", resp.Status, slurp)))
	}
	var all map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return fmt.Errorf("error decoding trace-agent /debug/vars response: %v", err)
	}
	v, err := yaml.Marshal(all)
	if err != nil {
		return err
	}
	return fb.AddFile(f, v)
}

func getSystemProbeSocketPath() string {
	return pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
}

func getSystemProbeStats() ([]byte, error) {
	// TODO: (components) - Temporary until we can use the status component to extract the system probe status from it.
	stats := map[string]interface{}{}
	systemprobeStatus.GetStatus(stats, getSystemProbeSocketPath())
	sysProbeBuf, err := yaml.Marshal(stats["systemProbeStats"])
	if err != nil {
		return nil, err
	}

	return sysProbeBuf, nil
}

func getSystemProbeTelemetry() ([]byte, error) {
	return sysprobe.GetSystemProbeTelemetry(getSystemProbeSocketPath())
}
func getSystemProbeConntrackCached() ([]byte, error) {
	return sysprobe.GetSystemProbeConntrackCached(getSystemProbeSocketPath())
}
func getSystemProbeConntrackHost() ([]byte, error) {
	return sysprobe.GetSystemProbeConntrackHost(getSystemProbeSocketPath())
}
func getSystemProbeBTFLoaderInfo() ([]byte, error) {
	return sysprobe.GetSystemProbeBTFLoaderInfo(getSystemProbeSocketPath())
}

// getProcessAgentFullConfig fetches process-agent runtime config as YAML and returns it to be added to  process_agent_runtime_config_dump.yaml
func getProcessAgentFullConfig() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	procStatusURL := fmt.Sprintf("http://%s/config/all", addressPort)

	bytes, err := getHTTPCallContent(procStatusURL)
	if err != nil {
		return []byte("error: process-agent is not running or is unreachable\n"), nil
	}
	return bytes, nil
}

func getConfigFiles(fb flaretypes.FlareBuilder, confSearchPaths map[string]string) {
	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool { //nolint:errcheck
			// ignore .example file
			if filepath.Ext(path) == ".example" {
				return false
			}

			firstSuffix := []byte(getFirstSuffix(path))
			ext := []byte(filepath.Ext(path))
			if cnfFileExtRx.Match(firstSuffix) || cnfFileExtRx.Match(ext) {
				return true
			}
			return false
		})
	}

	if pkgconfigsetup.Datadog().ConfigFileUsed() != "" {
		mainConfpath := pkgconfigsetup.Datadog().ConfigFileUsed()
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml")) //nolint:errcheck

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml")) //nolint:errcheck

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "security-agent.yaml")) //nolint:errcheck
	}
}

func getChecksFromProcessAgent(fb flaretypes.FlareBuilder, getAddressPort func() (url string, err error)) {
	addressPort, err := getAddressPort()
	if err != nil {
		log.Errorf("Could not zip process agent checks: wrong configuration to connect to process-agent: %s", err.Error())
		return
	}
	checkURL := fmt.Sprintf("http://%s/check/", addressPort)

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

	return getTaggerList(taggerListURL)
}

func getProcessAgentTaggerList() ([]byte, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	taggerListURL := fmt.Sprintf("http://%s/agent/tagger-list", addressPort)
	return getTaggerList(taggerListURL)
}

func getTaggerList(remoteURL string) ([]byte, error) {
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

	return getWorkloadList(fmt.Sprintf("https://%v:%v/agent/workload-list?verbose=true", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port")))
}

func getWorkloadList(url string) ([]byte, error) {
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

	client := http.Client{}
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

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
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
