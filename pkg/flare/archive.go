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

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	v5 "github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	host "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"gopkg.in/yaml.v2"
)

var (
	pprofURL = fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		config.Datadog.GetString("expvar_port"))
	telemetryURL = fmt.Sprintf("http://127.0.0.1:%s/telemetry",
		config.Datadog.GetString("expvar_port"))

	// Match .yaml and .yml to ship configuration files in the flare.
	cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)
)

// SearchPaths is just an alias for a map of strings
type SearchPaths map[string]string

// ProfileData maps (pprof) profile names to the profile data.
type ProfileData map[string][]byte

// CreatePerformanceProfile adds a set of heap and CPU profiles into target, using cpusec as the CPU
// profile duration, debugURL as the target URL for fetching the profiles and prefix as a prefix for
// naming them inside target.
//
// It is accepted to pass a nil target.
func CreatePerformanceProfile(prefix, debugURL string, cpusec int, target *ProfileData) error {
	c := apiutil.GetClient(false)
	if *target == nil {
		*target = make(ProfileData)
	}
	for _, prof := range []struct{ Name, URL string }{
		{
			// 1st heap profile
			Name: prefix + "-1st-heap.pprof",
			URL:  debugURL + "/heap",
		},
		{
			// CPU profile
			Name: prefix + "-cpu.pprof",
			URL:  fmt.Sprintf("%s/profile?seconds=%d", debugURL, cpusec),
		},
		{
			// 2nd heap profile
			Name: prefix + "-2nd-heap.pprof",
			URL:  debugURL + "/heap",
		},
		{
			// A sampling of all past memory allocations
			Name: prefix + "-allocs.pprof",
			URL:  debugURL + "/allocs",
		},
		{
			// mutex profile
			Name: prefix + "-mutex.pprof",
			URL:  debugURL + "/mutex",
		},
		{
			// goroutine blocking profile
			Name: prefix + "-block.pprof",
			URL:  debugURL + "/block",
		},
	} {
		b, err := apiutil.DoGet(c, prof.URL, apiutil.LeaveConnectionOpen)
		if err != nil {
			return err
		}
		(*target)[prof.Name] = b
	}
	return nil
}

// CreateArchive packages up the files
func CreateArchive(local bool, distPath, pyChecksPath string, logFilePaths []string, pdata ProfileData, ipcError error) (string, error) {
	fb, err := flarehelpers.NewFlareBuilder()
	if err != nil {
		return "", err
	}

	CompleteFlare(fb, local, distPath, pyChecksPath, logFilePaths, pdata, ipcError)
	return fb.Save()
}

// CompleteFlare packages up the files with an already created builder. This is aimed to be used by the flare
// component while we migrate to a component architecture.
func CompleteFlare(fb flarehelpers.FlareBuilder, local bool, distPath, pyChecksPath string, logFilePaths []string, pdata ProfileData, ipcError error) {
	confSearchPaths := SearchPaths{
		"":        config.Datadog.GetString("confd_path"),
		"dist":    filepath.Join(distPath, "conf.d"),
		"checksd": pyChecksPath,
	}
	createArchive(fb, confSearchPaths, local, logFilePaths, pdata, ipcError)
}

func createArchive(fb flarehelpers.FlareBuilder, confSearchPaths SearchPaths, local bool, logFilePaths []string, pdata ProfileData, ipcError error) {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze what is being added and ensure that it contains no credentials
	 * or unnecessary user-specific data. The FlareBuilder scrubs secrets that match pre-programmed patterns, but it
	 * is always better to not capture data containing secrets, than to scrub that data.
	 */

	if local {
		fb.AddFile("local", []byte(""))

		if ipcError != nil {
			// Can't reach the agent, mention it in those two files
			msg := []byte(fmt.Sprintf("unable to contact the agent to retrieve flare: %s", ipcError))
			fb.AddFile("status.log", msg)
			fb.AddFile("config-check.log", msg)
		} else {
			// Can't reach the agent, mention it in those two files
			fb.AddFile("status.log", []byte("unable to get the status of the agent, is it running?"))
			fb.AddFile("config-check.log", []byte("unable to get loaded checks config, is the agent running?"))
		}
	} else {
		// Status information are available, add them as the agent is running.

		fb.AddFileFromFunc("status.log", status.GetAndFormatStatus)
		fb.AddFileFromFunc("config-check.log", getConfigCheck)
		fb.AddFileFromFunc("tagger-list.json", getAgentTaggerList)
		fb.AddFileFromFunc("workload-list.log", getAgentWorkloadList)
		fb.AddFileFromFunc("process-agent_tagger-list.json", getProcessAgentTaggerList)

		getProcessChecks(fb, config.GetProcessAPIAddressPort)
	}

	fb.RegisterFilePerm(security.GetAuthTokenFilepath())

	systemProbeConfigBPFDir := config.Datadog.GetString("system_probe_config.bpf_dir")
	if systemProbeConfigBPFDir != "" {
		fb.RegisterDirPerm(systemProbeConfigBPFDir)
	}
	addSystemProbePlatformSpecificEntries(fb)

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		fb.AddFileFromFunc(filepath.Join("expvar", "system-probe"), getSystemProbeStats)
	}

	fb.AddFileFromFunc("process_agent_runtime_config_dump.yaml", getProcessAgentFullConfig)
	fb.AddFileFromFunc("runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(config.Datadog.AllSettings()) })
	fb.AddFileFromFunc("system_probe_runtime_config_dump.yaml", func() ([]byte, error) { return yaml.Marshal(config.SystemProbe.AllSettings()) })
	fb.AddFileFromFunc("diagnose.log", func() ([]byte, error) { return functionOutputToBytes(diagnose.RunAll), nil })
	fb.AddFileFromFunc("connectivity.log", getDatadogConnectivity)
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
	fb.CopyFile(filepath.Join(config.FileUsedDir(), "install_info"))

	getConfigFiles(fb, confSearchPaths)
	getExpVar(fb) //nolint:errcheck
	getWindowsData(fb)

	if config.Datadog.GetBool("telemetry.enabled") {
		fb.AddFileFromFunc("telemetry.log", func() ([]byte, error) { return getHTTPCallContent(telemetryURL) })
	}

	if config.Datadog.GetBool("remote_configuration.enabled") {
		if err := exportRemoteConfig(fb); err != nil {
			log.Errorf("Could not export remote-config state: %s", err)
		}
	}

	for _, logFilePath := range logFilePaths {
		getLogFiles(fb, logFilePath)
	}

	getPerformanceProfile(fb, pdata)
}

func getVersionHistory(fb flarehelpers.FlareBuilder) {
	fb.CopyFile(filepath.Join(config.Datadog.GetString("run_path"), "version-history.json"))
}

func getPerformanceProfile(fb flarehelpers.FlareBuilder, pdata ProfileData) {
	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data)
	}
}

func getRegistryJSON(fb flarehelpers.FlareBuilder) {
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

func getLogFiles(fb flarehelpers.FlareBuilder, logFileDir string) {
	log.Flush()

	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFileDir), "logs", func(path string) bool {
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	})
}

func getExpVar(fb flarehelpers.FlareBuilder) error {
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

	apmPort := "8126"
	if config.Datadog.IsSet("apm_config.receiver_port") {
		apmPort = config.Datadog.GetString("apm_config.receiver_port")
	}
	f := filepath.Join("expvar", "trace-agent")
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/debug/vars", apmPort))
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

func getConfigFiles(fb flarehelpers.FlareBuilder, confSearchPaths SearchPaths) {
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

func getProcessChecks(fb flarehelpers.FlareBuilder, getAddressPort func() (url string, err error)) {
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

func getDatadogConnectivity() ([]byte, error) {
	fct := func(w io.Writer) error {
		return connectivity.RunDatadogConnectivityDiagnose(w, false)
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
