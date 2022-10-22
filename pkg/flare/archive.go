// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/mholt/archiver/v3"
	"gopkg.in/yaml.v2"
)

const (
	routineDumpFilename = "go-routine-dump.log"

	// Maximum size for the root directory name
	directoryNameMaxSize = 32
)

var (
	pprofURL = fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/goroutine?debug=2",
		config.Datadog.GetString("expvar_port"))
	telemetryURL = fmt.Sprintf("http://127.0.0.1:%s/telemetry",
		config.Datadog.GetString("expvar_port"))
	procStatusURL string

	// Match .yaml and .yml to ship configuration files in the flare.
	cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

	// Filter to clean the directory name from invalid file name characters
	directoryNameFilter = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

	// specialized scrubber for flare content
	flareScrubber *scrubber.Scrubber
)

// SearchPaths is just an alias for a map of strings
type SearchPaths map[string]string

// permissionsInfos holds permissions info about the files shipped
// in the flare.
// The key is the filepath of the file.
type permissionsInfos map[string]filePermsInfo

type filePermsInfo struct {
	mode  os.FileMode
	owner string
	group string
}

// ProfileData maps (pprof) profile names to the profile data.
type ProfileData map[string][]byte

func init() {
	flareScrubber = scrubber.New()
	scrubber.AddDefaultReplacers(flareScrubber)

	// The default scrubber doesn't deal with api keys of other services, for
	// example powerDNS which has an "api_key" field in its YAML configuration.
	// We add a replacer to scrub even those credentials.
	//
	// It is a best effort to match the api key field without matching our
	// own already scrubbed (we don't want to match: "**************************abcde")
	// Basically we allow many special chars while forbidding *.
	//
	// We want the value to be at least 2 characters which will avoid matching the first '"' from the regular
	// replacer for api_key.
	otherAPIKeysRx := regexp.MustCompile(`api_key\s*:\s*[a-zA-Z0-9\\\/\^\]\[\(\){}!|%:;"~><=#@$_\-\+]{2,}`)
	flareScrubber.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: otherAPIKeysRx,
		ReplFunc: func(b []byte) []byte {
			return []byte("api_key: \"********\"")
		},
	})
}

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
	zipFilePath := getArchivePath()
	confSearchPaths := SearchPaths{
		"":        config.Datadog.GetString("confd_path"),
		"dist":    filepath.Join(distPath, "conf.d"),
		"checksd": pyChecksPath,
	}
	return createArchive(confSearchPaths, local, zipFilePath, logFilePaths, pdata, ipcError)
}

func createArchive(confSearchPaths SearchPaths, local bool, zipFilePath string, logFilePaths []string, pdata ProfileData, ipcError error) (string, error) {
	/** WARNING
	 *
	 * When adding data to flares, carefully analyze the what is being added
	 * and ensure that it contains no credentials or unnecessary user-specific
	 * data.  The ./pkg/util/scrubber package can be useful for scrubbing
	 * secrets that match pre-programmed patterns, but it is always better to
	 * not capture data containing secrets, than to scrub that data.
	 */

	tempDir, err := createTempDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := hostnameUtil.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}

	hostname = cleanDirectoryName(hostname)

	permsInfos := make(permissionsInfos)

	if local {
		err = writeLocal(tempDir, hostname)
		if err != nil {
			return "", err
		}

		if ipcError != nil {
			msg := []byte(fmt.Sprintf("unable to contact the agent to retrieve flare: %s", ipcError))
			// Can't reach the agent, mention it in those two files
			err = writeStatusFile(tempDir, hostname, msg)
			if err != nil {
				return "", err
			}
			err = writeConfigCheck(tempDir, hostname, msg)
			if err != nil {
				return "", err
			}
		} else {
			// Can't reach the agent, mention it in those two files
			err = writeStatusFile(tempDir, hostname, []byte("unable to get the status of the agent, is it running?"))
			if err != nil {
				return "", err
			}
			err = writeConfigCheck(tempDir, hostname, []byte("unable to get loaded checks config, is the agent running?"))
			if err != nil {
				return "", err
			}
		}
	} else {
		// Status informations are available, zip them up as the agent is running.
		err = zipStatusFile(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip status: %s", err)
		}

		err = zipConfigCheck(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip config check: %s", err)
		}

		err = zipAgentTaggerList(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip tagger list: %s", err)
		}

		err = zipWorkloadList(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip workload list: %s", err)
		}

		err = zipProcessAgentTaggerList(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip process-agent tagger list: %s", err)
		}

		err = zipProcessChecks(tempDir, hostname, config.GetProcessAPIAddressPort)
		if err != nil {
			log.Errorf("Could not zip process agent checks: %s", err)
		}
	}

	// auth token permissions info (only if existing)
	if _, err = os.Stat(security.GetAuthTokenFilepath()); err == nil && !os.IsNotExist(err) {
		permsInfos.add(security.GetAuthTokenFilepath())
	}

	err = zipConfigFiles(tempDir, hostname, confSearchPaths, permsInfos)
	if err != nil {
		log.Errorf("Could not zip config: %s", err)
	}

	err = zipExpVar(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip exp var: %s", err)
	}

	if config.SystemProbe.GetBool("system_probe_config.enabled") {
		err = zipSystemProbeStats(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip system probe exp var stats: %s", err)
		}
	}

	err = zipDiagnose(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip diagnose: %s", err)
	}

	err = zipDatadogConnectivity(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip diagnose datadog-connectivity: %s", err)
	}

	err = zipRegistryJSON(tempDir, hostname)
	if err != nil {
		log.Warnf("Could not zip registry.json: %s", err)
	}

	err = zipVersionHistory(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip version-history.json: %s", err)
	}

	err = zipSecrets(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip secrets: %s", err)
	}

	err = zipEnvvars(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip env vars: %s", err)
	}

	err = zipMetadataInventories(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip inventories metadata payload: %s", err)
	}

	err = zipMetadataV5(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip v5 metadata payload: %s", err)
	}

	err = zipHealth(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip health check: %s", err)
	}

	if config.Datadog.GetBool("telemetry.enabled") {
		err = zipTelemetry(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not collect telemetry metrics: %s", err)
		}
	}

	err = zipStackTraces(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not collect go routine stack traces: %s", err)
	}

	if config.IsContainerized() {
		err = zipDockerSelfInspect(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip docker inspect: %s", err)
		}
	}

	err = zipDockerPs(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip docker ps: %s", err)
	}

	err = zipTypeperfData(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not write typeperf data: %s", err)
	}
	err = zipLodctrOutput(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not write lodctr data: %s", err)
	}

	err = zipCounterStrings(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not write counter strings: %s", err)
	}

	err = zipWindowsEventLogs(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not export Windows event logs: %s", err)
	}
	err = zipServiceStatus(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not export Windows driver status: %s", err)
	}
	err = zipDatadogRegistry(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not export Windows Datadog Registry: %s", err)
	}

	if config.Datadog.GetBool("remote_configuration.enabled") {
		err = zipRemoteConfigDB(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not export remote-config database: %s", err)
		}
	}

	// force a log flush before zipping them
	log.Flush()
	for _, logFilePath := range logFilePaths {
		err = zipLogFiles(tempDir, hostname, logFilePath, permsInfos)
		if err != nil {
			log.Errorf("Could not zip logs: %s", err)
		}
	}

	err = zipInstallInfo(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip install_info: %s", err)
	}

	if pdata != nil {
		err = zipPerformanceProfile(tempDir, hostname, pdata)
		if err != nil {
			log.Errorf("Could not zip performance profile: %s", err)
		}
	}

	// gets files infos and write the permissions.log file
	if err := permsInfos.commit(tempDir, hostname, os.ModePerm); err != nil {
		log.Errorf("Could not write permissions.log file: %s", err)
	}

	// File format is determined based on `zipFilePath` extension
	err = archiver.Archive([]string{filepath.Join(tempDir, hostname)}, zipFilePath)
	if err != nil {
		return "", err
	}

	return zipFilePath, nil
}

func createTempDir() (string, error) {
	b := make([]byte, 10)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	dirName := hex.EncodeToString(b)
	return ioutil.TempDir("", dirName)
}

func zipStatusFile(tempDir, hostname string) error {
	// Grab the status
	s, err := status.GetAndFormatStatus()
	if err != nil {
		return err
	}
	return writeStatusFile(tempDir, hostname, s)
}

func writeStatusFile(tempDir, hostname string, data []byte) error {
	return joinPathAndWriteScrubbedFile(data, tempDir, hostname, "status.log")
}

func addParentPerms(dirPath string, permsInfos permissionsInfos) {
	parent := filepath.Dir(dirPath)

	// We do not enter the loop when `filepath.Dir` returns ".", meaning an empty directory was passed.
	for parent != "." {
		if len(filepath.Dir(parent)) == len(parent) {
			permsInfos.add(parent)
			break
		}

		permsInfos.add(parent)
		parent = filepath.Dir(parent)
	}
}

func zipLogFiles(tempDir, hostname, logFilePath string, permsInfos permissionsInfos) error {
	// Force dir path to be absolute first
	logFileDir, err := filepath.Abs(filepath.Dir(logFilePath))
	if err != nil {
		log.Errorf("Error getting absolute path to log directory of %q: %v", logFilePath, err)
		return err
	}
	permsInfos.add(logFileDir)

	err = filepath.Walk(logFileDir, func(src string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".log" || getFirstSuffix(f.Name()) == ".log" {
			targRelPath, relErr := filepath.Rel(logFileDir, src)
			if relErr != nil {
				log.Errorf("Can't get relative path to %q: %v", src, relErr)
				return nil
			}
			dst := filepath.Join(tempDir, hostname, "logs", targRelPath)

			if permsInfos != nil {
				permsInfos.add(src)
			}

			return util.CopyFileAll(src, dst)
		}
		return nil
	})

	// The permsInfos map is empty when we cannot read the auth token.
	if len(permsInfos) != 0 {
		addParentPerms(logFileDir, permsInfos)
	}

	return err
}

func zipExpVar(tempDir, hostname string) error {
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

		err = joinPathAndWriteScrubbedFile(yamlValue, tempDir, hostname, "expvar", key)
		if err != nil {
			return err
		}
	}

	apmPort := "8126"
	if config.Datadog.IsSet("apm_config.receiver_port") {
		apmPort = config.Datadog.GetString("apm_config.receiver_port")
	}
	f := filepath.Join(tempDir, hostname, "expvar", "trace-agent")
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/debug/vars", apmPort))
	if err != nil {
		return writeScrubbedFile(f, []byte(fmt.Sprintf("Error retrieving vars: %v", err)))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return writeScrubbedFile(f,
			[]byte(fmt.Sprintf("Got response %s from /debug/vars:\n%s", resp.Status, slurp)))
	}
	var all map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return fmt.Errorf("error decoding trace-agent /debug/vars response: %v", err)
	}
	v, err := yaml.Marshal(all)
	if err != nil {
		return err
	}
	return writeScrubbedFile(f, v)
}

func zipSystemProbeStats(tempDir, hostname string) error {
	sysProbeStats := status.GetSystemProbeStats(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	sysProbeBuf, err := yaml.Marshal(sysProbeStats)
	if err != nil {
		return err
	}

	return joinPathAndWriteScrubbedFile(sysProbeBuf, tempDir, hostname, "expvar", "system-probe")
}

// zipProcessAgentFullConfig fetches process-agent runtime config as YAML and writes it to process_agent_runtime_config_dump.yaml
func zipProcessAgentFullConfig(tempDir, hostname string) error {
	// procStatusURL can be manually set for test purposes
	if procStatusURL == "" {
		addressPort, err := config.GetProcessAPIAddressPort()
		if err != nil {
			return fmt.Errorf("wrong configuration to connect to process-agent")
		}

		procStatusURL = fmt.Sprintf("http://%s/config/all", addressPort)
	}

	cfgB := status.GetProcessAgentRuntimeConfig(procStatusURL)

	return joinPathAndWriteScrubbedFile(cfgB, tempDir, hostname, "process_agent_runtime_config_dump.yaml")
}

func zipConfigFiles(tempDir, hostname string, confSearchPaths SearchPaths, permsInfos permissionsInfos) error {
	c, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return err
	}

	err = joinPathAndWriteScrubbedFile(c, tempDir, hostname, "runtime_config_dump.yaml")
	if err != nil {
		return err
	}

	// Use best effort to write process-agent runtime configs
	err = zipProcessAgentFullConfig(tempDir, hostname)
	if err != nil {
		log.Warnf("could not zip process_agent_runtime_config_dump.yaml: %s", err)
	}

	err = walkConfigFilePaths(tempDir, hostname, confSearchPaths, permsInfos)
	if err != nil {
		return err
	}

	if config.Datadog.ConfigFileUsed() != "" {
		// zip up the config file that was actually used, if one exists
		filePath := config.Datadog.ConfigFileUsed()
		if err = createConfigFiles(filePath, tempDir, hostname, permsInfos); err != nil {
			return err
		}
		// figure out system-probe file path based on main config path,
		// and use best effort to include system-probe.yaml to the flare
		systemProbePath := getConfigPath(filePath, "system-probe.yaml")
		if systemErr := createConfigFiles(systemProbePath, tempDir, hostname, permsInfos); systemErr != nil {
			log.Warnf("could not zip system-probe.yaml, system-probe might not be configured, or is in a different directory with datadog.yaml: %s", systemErr)
		}

		// use best effort to include security-agent.yaml to the flare
		securityAgentPath := getConfigPath(filePath, "security-agent.yaml")
		if secErr := createConfigFiles(securityAgentPath, tempDir, hostname, permsInfos); secErr != nil {
			log.Warnf("could not zip security-agent.yaml, security-agent might not be configured, or is in a different directory with datadog.yaml: %s", secErr)
		}
	}

	return err
}

func zipSecrets(tempDir, hostname string) error {
	fct := func(writer io.Writer) error {
		info, err := secrets.GetDebugInfo()
		if err != nil {
			fmt.Fprintf(writer, "%s", err)
		} else {
			info.Print(writer)
		}
		return nil
	}

	return zipCommandOutput(fct, tempDir, hostname, "secrets.log")
}

func zipProcessChecks(tempDir, hostname string, getAddressPort func() (url string, err error)) error {
	addressPort, err := getAddressPort()
	if err != nil {
		return fmt.Errorf("wrong configuration to connect to process-agent: %s", err.Error())
	}
	checkURL := fmt.Sprintf("http://%s/check/", addressPort)

	zipCheck := func(checkName, setting string) error {
		if !config.Datadog.GetBool(setting) {
			return nil
		}

		filename := fmt.Sprintf("%s_check_output.json", checkName)
		if err := zipHTTPCallContent(tempDir, hostname, filename, checkURL+checkName); err != nil {
			_ = log.Error(err)
			err = ioutil.WriteFile(
				filepath.Join(tempDir, hostname, "process_check_output.json"),
				[]byte(fmt.Sprintf("error: process-agent is not running or is unreachable: %s", err.Error())),
				os.ModePerm,
			)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if err := zipCheck("process", "process_config.process_collection.enabled"); err != nil {
		return err
	}

	if err := zipCheck("container", "process_config.container_collection.enabled"); err != nil {
		return err
	}

	if err := zipCheck("process_discovery", "process_config.process_discovery.enabled"); err != nil {
		return err
	}

	return nil
}

func zipDiagnose(tempDir, hostname string) error {
	return zipCommandOutput(diagnose.RunAll, tempDir, hostname, "diagnose.log")
}

func zipDatadogConnectivity(tempDir, hostname string) error {
	fct := func(w io.Writer) error {
		return connectivity.RunDatadogConnectivityDiagnose(w, false)
	}
	return zipCommandOutput(fct, tempDir, hostname, "connectivity.log")
}

func zipCommandOutput(fct func(w io.Writer) error, tempDir, hostname, filename string) error {
	bytes := functionOutputToBytes(fct)
	return joinPathAndWriteScrubbedFile(bytes, tempDir, hostname, filename)
}

func zipReader(r io.Reader, targetDir, filename string) error {
	targetPath := filepath.Join(targetDir, filename)

	if err := ensureParentDirsExist(targetPath); err != nil {
		return err
	}

	zipped, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer zipped.Close()

	// use read/write instead of io.Copy
	// see: https://github.com/golang/go/issues/44272
	buf := make([]byte, 256)
	for {
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := zipped.Write(buf[:n]); err != nil {
			return err
		}
	}
	return err
}

func zipFile(sourceDir, targetDir, filename string) error {
	original, err := os.Open(filepath.Join(sourceDir, filename))
	if err != nil {
		return err
	}
	defer original.Close()

	return zipReader(original, targetDir, filename)
}

func zipRegistryJSON(tempDir, hostname string) error {
	originalPath := config.Datadog.GetString("logs_config.run_path")
	targetPath := filepath.Join(tempDir, hostname)
	return zipFile(originalPath, targetPath, "registry.json")
}

func zipVersionHistory(tempDir, hostname string) error {
	originalPath := config.Datadog.GetString("run_path")
	targetPath := filepath.Join(tempDir, hostname)
	return zipFile(originalPath, targetPath, "version-history.json")
}

func zipConfigCheck(tempDir, hostname string) error {
	fct := func(w io.Writer) error {
		return GetConfigCheck(w, true)
	}
	bytes := functionOutputToBytes(fct)
	return writeConfigCheck(tempDir, hostname, bytes)
}

func writeConfigCheck(tempDir, hostname string, data []byte) error {
	return joinPathAndWriteScrubbedFile(data, tempDir, hostname, "config-check.log")
}

// Used for testing mock HTTP server
var taggerListURL string

func zipAgentTaggerList(tempDir, hostname string) error {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}

	if taggerListURL == "" {
		taggerListURL = fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}

	return zipTaggerList(tempDir, hostname, taggerListURL, "tagger-list.json")
}

func zipProcessAgentTaggerList(tempDir, hostname string) error {
	addressPort, err := config.GetProcessAPIAddressPort()
	if err != nil {
		return fmt.Errorf("wrong configuration to connect to process-agent")
	}

	taggerListURL := fmt.Sprintf("http://%s/agent/tagger-list", addressPort)

	return zipTaggerList(tempDir, hostname, taggerListURL, "process-agent_tagger-list.json")
}

func zipTaggerList(tempDir, hostname, remoteURL, outputFile string) error {
	c := apiutil.GetClient(false) // FIX: get certificates right then make this true

	r, err := apiutil.DoGet(c, remoteURL, apiutil.LeaveConnectionOpen)
	if err != nil {
		return err
	}

	// Pretty print JSON output
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	err = json.Indent(&b, r, "", "\t")
	if err != nil {
		return joinPathAndWriteScrubbedFile(r, tempDir, hostname, outputFile)
	}
	writer.Flush()

	return joinPathAndWriteScrubbedFile(b.Bytes(), tempDir, hostname, outputFile)
}

// workloadListURL allows mocking the agent HTTP server
var workloadListURL string

func zipWorkloadList(tempDir, hostname string) error {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}

	if workloadListURL == "" {
		workloadListURL = fmt.Sprintf("https://%v:%v/agent/workload-list/verbose", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}

	c := apiutil.GetClient(false) // FIX: get certificates right then make this true

	r, err := apiutil.DoGet(c, workloadListURL, apiutil.LeaveConnectionOpen)
	if err != nil {
		return err
	}

	workload := workloadmeta.WorkloadDumpResponse{}
	err = json.Unmarshal(r, &workload)
	if err != nil {
		return err
	}

	fct := func(w io.Writer) error {
		workload.Write(w)
		return nil
	}
	return zipCommandOutput(fct, tempDir, hostname, "workload-list.log")
}

func zipHealth(tempDir, hostname string) error {
	s := health.GetReady()
	sort.Strings(s.Healthy)
	sort.Strings(s.Unhealthy)

	yamlValue, err := yaml.Marshal(s)
	if err != nil {
		return err
	}

	return joinPathAndWriteScrubbedFile(yamlValue, tempDir, hostname, "health.yaml")
}

func zipInstallInfo(tempDir, hostname string) error {
	targetPath := filepath.Join(tempDir, hostname)
	return zipFile(config.FileUsedDir(), targetPath, "install_info")
}

func zipTelemetry(tempDir, hostname string) error {
	return zipHTTPCallContent(tempDir, hostname, "telemetry.log", telemetryURL)
}

func zipStackTraces(tempDir, hostname string) error {
	return zipHTTPCallContent(tempDir, hostname, routineDumpFilename, pprofURL)
}

// zipHTTPCallContent does a GET HTTP call to the given url and
// writes the content of the HTTP response in the given file, ready
// to be shipped in a flare.
func zipHTTPCallContent(tempDir, hostname, filename, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	client := http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// read the entire body, so that it can be scrubbed in its entirety
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return joinPathAndWriteScrubbedFile(data, tempDir, hostname, filename)
}

func zipPerformanceProfile(tempDir, hostname string, pdata ProfileData) error {
	dir := filepath.Join(tempDir, hostname, "profiles")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	for name, data := range pdata {
		fullpath := filepath.Join(dir, name)
		if err := ioutil.WriteFile(fullpath, data, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

func walkConfigFilePaths(tempDir, hostname string, confSearchPaths SearchPaths, permsInfos permissionsInfos) error {
	for prefix, filePath := range confSearchPaths {

		err := filepath.Walk(filePath, func(src string, f os.FileInfo, err error) error {
			if f == nil {
				return nil
			}
			if f.IsDir() {
				return nil
			}

			if filepath.Ext(f.Name()) == ".example" {
				return nil
			}

			firstSuffix := getFirstSuffix(f.Name())
			ext := filepath.Ext(f.Name())

			if cnfFileExtRx.Match([]byte(firstSuffix)) || cnfFileExtRx.Match([]byte(ext)) {
				baseName := strings.Replace(src, filePath, "", 1)

				data, err := ioutil.ReadFile(src)
				if err != nil {
					if os.IsNotExist(err) {
						log.Warnf("the specified path: %s does not exist", filePath)
					}
					return err
				}
				err = joinPathAndWriteScrubbedFile(data, tempDir, hostname, "etc", "confd", prefix, baseName)
				if err != nil {
					return err
				}

				if permsInfos != nil {
					permsInfos.add(src)

					if len(permsInfos) != 0 {
						absPath, err := filepath.Abs(filePath)
						if err != nil {
							log.Errorf("Error while getting absolute file path for parent directory: %v", err)
						}
						addParentPerms(absPath, permsInfos)
					}
				}
			}

			return nil
		})
		if err != nil {
			return err
		}

	}

	return nil
}

// writeScrubbedFile writes the given data to the given file, after applying
// flareScrubber to it.
//
// WARNING: while this function applies a scrubber, that scrubber cannot scrub
// all secrets.  Ensure that the data being written cannot contain user secrets
// or proprietary information. For example, do not include arbitrary
// environment variables.
func writeScrubbedFile(filename string, data []byte) error {
	scrubbed, err := flareScrubber.ScrubBytes(data)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, scrubbed, os.ModePerm)
}

// writeScrubbedFileSafe verifies that the folder exists before calling writeScrubbedFile()
func writeScrubbedFileSafe(filename string, data []byte) error {
	err := ensureParentDirsExist(filename)
	if err != nil {
		return err
	}
	return writeScrubbedFile(filename, data)
}

// joinPathAndWriteScrubbedFile joins all the path components into a path and safely
// writes data to the created path
func joinPathAndWriteScrubbedFile(data []byte, pathComponents ...string) error {
	filename := filepath.Join(pathComponents...)
	return writeScrubbedFileSafe(filename, data)
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

func ensureParentDirsExist(p string) error {
	return os.MkdirAll(filepath.Dir(p), os.ModePerm)
}

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func getArchivePath() string {
	dir := os.TempDir()
	t := time.Now().UTC()
	timeString := strings.ReplaceAll(t.Format(time.RFC3339), ":", "-")
	fileName := ""
	lvl, err := log.GetLogLevel()
	if err == nil {
		fileName = strings.Join([]string{"datadog", "agent", timeString, lvl.String()}, "-")
	} else {
		fileName = strings.Join([]string{"datadog", "agent", timeString}, "-")
	}
	fileName = strings.Join([]string{fileName, "zip"}, ".")
	filePath := filepath.Join(dir, fileName)
	return filePath
}

func cleanDirectoryName(name string) string {
	filteredName := directoryNameFilter.ReplaceAllString(name, "_")
	if len(filteredName) > directoryNameMaxSize {
		return filteredName[:directoryNameMaxSize]
	}
	return filteredName
}

// createConfigFiles takes the content of config files that need to be included in the flare and
// put them in the directory waiting to be archived
func createConfigFiles(filePath, tempDir, hostname string, permsInfos permissionsInfos) error {
	// Check if the file exists
	_, err := os.Stat(filePath)
	if err == nil {

		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			return err
		}

		err = joinPathAndWriteScrubbedFile(data, tempDir, hostname, "etc", filepath.Base(filePath))
		if err != nil {
			return err
		}

		if permsInfos != nil {
			permsInfos.add(filePath)
		}
	}
	return err
}

// getConfigPath would take the path to datadog.yaml and replace the file name with the given agent file name
func getConfigPath(ddCfgFilePath string, agentFileName string) string {
	path := filepath.Dir(ddCfgFilePath)
	return filepath.Join(path, agentFileName)
}
