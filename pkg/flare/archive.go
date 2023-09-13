// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	v5 "github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	host "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"gopkg.in/yaml.v2"
)

var (
	// Match .yaml and .yml to ship configuration files in the flare.
	cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)
)

// searchPaths is a list of path where to look for checks configurations
type searchPaths map[string]string

// CompleteFlare packages up the files with an already created builder. This is aimed to be used by the flare
// component while we migrate to a component architecture.
func CompleteFlare(fb flaretypes.FlareBuilder, senderManager sender.SenderManager) error {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze what is being added and ensure that it contains no credentials
	 * or unnecessary user-specific data. The FlareBuilder scrubs secrets that match pre-programmed patterns, but it
	 * is always better to not capture data containing secrets, than to scrub that data.
	 */
	if fb.IsLocal() {
		// Can't reach the agent, mention it in those two files
		fb.AddFile("status.log", []byte("unable to get the status of the agent, is it running?"))
		fb.AddFile("config-check.log", []byte("unable to get loaded checks config, is the agent running?"))
	} else {
		fb.AddFileFromFunc("status.log", status.GetAndFormatStatus)
		fb.AddFileFromFunc("config-check.log", getConfigCheck)
		fb.AddFileFromFunc("tagger-list.json", getAgentTaggerList)
		fb.AddFileFromFunc("workload-list.log", getAgentWorkloadList)
		fb.AddFileFromFunc("process-agent_tagger-list.json", getProcessAgentTaggerList)

		getProcessChecks(fb, config.GetProcessAPIAddressPort)
	}

	fb.RegisterFilePerm(security.GetAuthTokenFilepath())

	systemProbeConfigBPFDir := config.SystemProbe.GetString("system_probe_config.bpf_dir")
	if systemProbeConfigBPFDir != "" {
		fb.RegisterDirPerm(systemProbeConfigBPFDir)
	}
	addSystemProbePlatformSpecificEntries(fb)

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		fb.AddFileFromFunc(filepath.Join("expvar", "system-probe"), getSystemProbeStats)
	}

	pprofURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		config.Datadog.GetString("expvar_port"))

	fb.AddFileFromFunc("process_agent_runtime_config_dump.yaml", getProcessAgentFullConfig)
	fb.AddFileFromFunc("runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(config.Datadog.AllSettings()) })
	fb.AddFileFromFunc("system_probe_runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(config.SystemProbe.AllSettings()) })
	fb.AddFileFromFunc("diagnose.log", func() ([]byte, error) { return getDiagnoses(senderManager) })
	fb.AddFileFromFunc("secrets.log", getSecrets)
	fb.AddFileFromFunc("envvars.log", getEnvVars)
	fb.AddFileFromFunc("metadata_inventories.json", inventories.GetLastPayload)
	fb.AddFileFromFunc("metadata_v5.json", getMetadataV5)
	fb.AddFileFromFunc("health.yaml", getHealth)
	fb.AddFileFromFunc("go-routine-dump.log", func() ([]byte, error) { return getHTTPCallContent(pprofURL) })
	fb.AddFileFromFunc("docker_inspect.log", getDockerSelfInspect)
	fb.AddFileFromFunc("docker_ps.log", getDockerPs)

	getRegistryJSON(fb)

	getVersionHistory(fb)
	fb.CopyFile(installinfo.GetFilePath(config.Datadog))

	getExpVar(fb) //nolint:errcheck
	getWindowsData(fb)

	if config.Datadog.GetBool("telemetry.enabled") {
		telemetryURL := fmt.Sprintf("http://127.0.0.1:%s/telemetry",
			config.Datadog.GetString("expvar_port"))
		fb.AddFileFromFunc("telemetry.log", func() ([]byte, error) { return getHTTPCallContent(telemetryURL) })
	}

	if config.IsRemoteConfigEnabled(config.Datadog) {
		if err := exportRemoteConfig(fb); err != nil {
			log.Errorf("Could not export remote-config state: %s", err)
		}
	}
	return nil
}

func getVersionHistory(fb flaretypes.FlareBuilder) {
	fb.CopyFile(filepath.Join(config.Datadog.GetString("run_path"), "version-history.json"))
}

func getRegistryJSON(fb flaretypes.FlareBuilder) {
	fb.CopyFile(filepath.Join(config.Datadog.GetString("logs_config.run_path"), "registry.json"))
}

func getMetadataV5() ([]byte, error) {
	ctx := context.Background()
	hostnameData, _ := host.GetWithProvider(ctx)
	payload := v5.GetPayload(ctx, hostnameData)

	data, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		return nil, err
	}

	return data, nil
}

func getLogFiles(fb flaretypes.FlareBuilder, logFileDir string) {
	log.Flush()

	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFileDir), "logs", func(path string) bool {
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

	apmDebugPort := config.Datadog.GetInt("apm_config.debug.port")
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

func getSystemProbeStats() ([]byte, error) {
	sysProbeStats := status.GetSystemProbeStats(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	sysProbeBuf, err := yaml.Marshal(sysProbeStats)
	if err != nil {
		return nil, err
	}

	return sysProbeBuf, nil
}

// getProcessAgentFullConfig fetches process-agent runtime config as YAML and returns it to be added to  process_agent_runtime_config_dump.yaml
func getProcessAgentFullConfig() ([]byte, error) {
	addressPort, err := config.GetProcessAPIAddressPort()
	if err != nil {
		return nil, fmt.Errorf("wrong configuration to connect to process-agent")
	}

	procStatusURL := fmt.Sprintf("http://%s/config/all", addressPort)

	cfgB := status.GetProcessAgentRuntimeConfig(procStatusURL)
	return cfgB, nil
}

func getConfigFiles(fb flaretypes.FlareBuilder, confSearchPaths map[string]string) {
	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool {
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

	if config.Datadog.ConfigFileUsed() != "" {
		mainConfpath := config.Datadog.ConfigFileUsed()
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml"))

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml"))

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "security-agent.yaml"))
	}
}

func getSecrets() ([]byte, error) {
	fct := func(writer io.Writer) error {
		secrets.GetDebugInfo(writer)
		return nil
	}

	return functionOutputToBytes(fct), nil
}

func getProcessChecks(fb flaretypes.FlareBuilder, getAddressPort func() (url string, err error)) {
	addressPort, err := getAddressPort()
	if err != nil {
		log.Errorf("Could not zip process agent checks: wrong configuration to connect to process-agent: %s", err.Error())
		return
	}
	checkURL := fmt.Sprintf("http://%s/check/", addressPort)

	getCheck := func(checkName, setting string) {
		filename := fmt.Sprintf("%s_check_output.json", checkName)

		if !config.Datadog.GetBool(setting) {
			fb.AddFile(filename, []byte(fmt.Sprintf("'%s' is disabled", setting)))
			return
		}

		err := fb.AddFileFromFunc(filename, func() ([]byte, error) { return getHTTPCallContent(checkURL + checkName) })
		if err != nil {
			fb.AddFile(
				"process_check_output.json",
				[]byte(fmt.Sprintf("error: process-agent is not running or is unreachable: %s", err.Error())),
			)
		}
	}

	getCheck("process", "process_config.process_collection.enabled")
	getCheck("container", "process_config.container_collection.enabled")
	getCheck("process_discovery", "process_config.process_discovery.enabled")
}

func getDiagnoses(senderManager sender.SenderManager) ([]byte, error) {
	fct := func(w io.Writer) error {
		// Run agent diagnose command to be verbose and remote. If Agent is running small performance hit
		// since this code will get diagnoses using Agent’s local port listener (instead of calling a
		// function directly since the caller and callee are in the same process).However, the same code
		// will continue to work well because agent diagnose command works locally as well (if it cannot
		// connect to the running Agent).
		diagCfg := diagnosis.Config{
			Verbose:  true,
			RunLocal: false,
		}

		return diagnose.RunStdOut(w, diagCfg, senderManager)
	}

	return functionOutputToBytes(fct), nil
}

func getConfigCheck() ([]byte, error) {
	fct := func(w io.Writer) error {
		return GetConfigCheck(w, true)
	}
	return functionOutputToBytes(fct), nil
}

func getAgentTaggerList() ([]byte, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.Datadog.GetInt("cmd_port"))

	return getTaggerList(taggerListURL)
}

func getProcessAgentTaggerList() ([]byte, error) {
	addressPort, err := config.GetProcessAPIAddressPort()
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
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	return getWorkloadList(fmt.Sprintf("https://%v:%v/agent/workload-list?verbose=true", ipcAddress, config.Datadog.GetInt("cmd_port")))
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
