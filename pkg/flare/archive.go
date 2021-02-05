// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/mholt/archiver"
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

	// Match .yaml and .yml to ship configuration files in the flare.
	cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

	// Filter to clean the directory name from invalid file name characters
	directoryNameFilter = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

	// Match other services api keys
	// It is a best effort to match the api key field without matching our
	// own already redacted (we don't want to match: **************************abcde)
	// Basically we allow many special chars while forbidding *
	otherAPIKeysRx       = regexp.MustCompile(`api_key\s*:\s*[a-zA-Z0-9\\\/\^\]\[\(\){}!|%:;"~><=#@$_\-\+]+`)
	otherAPIKeysReplacer = log.Replacer{
		Regex: otherAPIKeysRx,
		ReplFunc: func(b []byte) []byte {
			return []byte("api_key: ********")
		},
	}
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
	} {
		b, err := apiutil.DoGet(c, prof.URL)
		if err != nil {
			return err
		}
		(*target)[prof.Name] = b
	}
	return nil
}

// CreateArchive packages up the files
func CreateArchive(local bool, distPath, pyChecksPath string, logFilePaths []string, pdata ProfileData) (string, error) {
	zipFilePath := getArchivePath()
	confSearchPaths := SearchPaths{
		"":        config.Datadog.GetString("confd_path"),
		"dist":    filepath.Join(distPath, "conf.d"),
		"checksd": pyChecksPath,
	}
	return createArchive(confSearchPaths, local, zipFilePath, logFilePaths, pdata)
}

func createArchive(confSearchPaths SearchPaths, local bool, zipFilePath string, logFilePaths []string, pdata ProfileData) (string, error) {
	tempDir, err := createTempDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	// Get hostname, if there's an error in getting the hostname,
	// set the hostname to unknown
	hostname, err := util.GetHostname()
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
		// Can't reach the agent, mention it in those two files
		err = writeStatusFile(tempDir, hostname, []byte("unable to get the status of the agent, is it running?"))
		if err != nil {
			return "", err
		}
		err = writeConfigCheck(tempDir, hostname, []byte("unable to get loaded checks config, is the agent running?"))
		if err != nil {
			return "", err
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

		err = zipTaggerList(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip tagger list: %s", err)
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

	if config.Datadog.GetBool("system_probe_config.enabled") {
		err = zipSystemProbeStats(tempDir, hostname)
		if err != nil {
			log.Errorf("Could not zip system probe exp var stats: %s", err)
		}
	}

	err = zipDiagnose(tempDir, hostname)
	if err != nil {
		log.Errorf("Could not zip diagnose: %s", err)
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

	err = archiver.Zip.Make(zipFilePath, []string{filepath.Join(tempDir, hostname)})
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
	f := filepath.Join(tempDir, hostname, "status.log")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(data)
	return err
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
	logFileDir := filepath.Dir(logFilePath)

	err := filepath.Walk(logFileDir, func(src string, f os.FileInfo, err error) error {
		if f == nil {
			return nil
		}
		if f.IsDir() {
			return nil
		}

		if filepath.Ext(f.Name()) == ".log" || getFirstSuffix(f.Name()) == ".log" {
			dst := filepath.Join(tempDir, hostname, "logs", f.Name())

			if permsInfos != nil {
				permsInfos.add(src)
			}

			return util.CopyFileAll(src, dst)
		}
		return nil
	})

	// The permsInfos map is empty when we cannot read the auth token.
	if len(permsInfos) != 0 {
		// Force path to be absolute for getting parent permissions.
		absPath, err := filepath.Abs(logFileDir)
		if err != nil {
			log.Errorf("Error while getting absolute file path for parent directory: %v", err)
		}
		addParentPerms(absPath, permsInfos)
	}

	return err
}

func zipExpVar(tempDir, hostname string) error {
	var variables = make(map[string]interface{})
	expvar.Do(func(kv expvar.KeyValue) {
		var variable = make(map[string]interface{})
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

		f := filepath.Join(tempDir, hostname, "expvar", key)
		err = ensureParentDirsExist(f)
		if err != nil {
			return err
		}

		w, err := newRedactingWriter(f, os.ModePerm, true)
		if err != nil {
			return err
		}
		defer w.Close()

		_, err = w.Write(yamlValue)
		if err != nil {
			return err
		}
	}

	apmPort := "8126"
	if config.Datadog.IsSet("apm_config.receiver_port") {
		apmPort = config.Datadog.GetString("apm_config.receiver_port")
	}
	f := filepath.Join(tempDir, hostname, "expvar", "trace-agent")
	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/debug/vars", apmPort))
	if err != nil {
		_, err := w.Write([]byte(fmt.Sprintf("Error retrieving vars: %v", err)))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(fmt.Sprintf("Got response %s from /debug/vars:\n%s", resp.Status, string(slurp))))
		return err
	}
	var all map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return fmt.Errorf("error decoding trace-agent /debug/vars response: %v", err)
	}
	v, err := yaml.Marshal(all)
	if err != nil {
		return err
	}
	_, err = w.Write(v)
	return err
}

func zipSystemProbeStats(tempDir, hostname string) error {
	sysProbeStats := status.GetSystemProbeStats(config.Datadog.GetString("system_probe_config.sysprobe_socket"))
	sysProbeFile := filepath.Join(tempDir, hostname, "expvar", "system-probe")
	sysProbeWriter, err := newRedactingWriter(sysProbeFile, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer sysProbeWriter.Close()

	sysProbeBuf, err := yaml.Marshal(sysProbeStats)
	if err != nil {
		return err
	}
	_, err = sysProbeWriter.Write(sysProbeBuf)
	return err
}

func zipConfigFiles(tempDir, hostname string, confSearchPaths SearchPaths, permsInfos permissionsInfos) error {
	c, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return err
	}

	f := filepath.Join(tempDir, hostname, "runtime_config_dump.yaml")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(c)
	if err != nil {
		return err
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
		systemProbePath := getSystemProbePath(filePath)
		if systemErr := createConfigFiles(systemProbePath, tempDir, hostname, permsInfos); systemErr != nil {
			log.Warnf("could not zip system-probe.yaml, system-probe might not be configured, or is in a different directory with datadog.yaml: %s", systemErr)
		}
	}

	return err
}

func zipSecrets(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	info, err := secrets.GetDebugInfo()
	if err != nil {
		fmt.Fprintf(writer, "%s", err)
	} else {
		info.Print(writer)
	}
	writer.Flush()

	f := filepath.Join(tempDir, hostname, "secrets.log")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(b.Bytes())
	return err
}

func zipDiagnose(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	diagnose.RunAll(writer) //nolint:errcheck
	writer.Flush()

	f := filepath.Join(tempDir, hostname, "diagnose.log")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(b.Bytes())
	return err
}

func zipRegistryJSON(tempDir, hostname string) error {
	originalPath := filepath.Join(config.Datadog.GetString("logs_config.run_path"), "registry.json")
	original, err := os.Open(originalPath)
	if err != nil {
		return err
	}
	defer original.Close()

	zippedPath := filepath.Join(tempDir, hostname, "registry.json")
	err = ensureParentDirsExist(zippedPath)
	if err != nil {
		return err
	}

	zipped, err := os.OpenFile(zippedPath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer zipped.Close()

	_, err = io.Copy(zipped, original)
	return err
}

func zipVersionHistory(tempDir, hostname string) error {
	originalPath := filepath.Join(config.Datadog.GetString("run_path"), "version-history.json")
	original, err := os.Open(originalPath)
	if err != nil {
		return err
	}
	defer original.Close()

	zippedPath := filepath.Join(tempDir, hostname, "version-history.json")
	err = ensureParentDirsExist(zippedPath)
	if err != nil {
		return err
	}

	zipped, err := os.OpenFile(zippedPath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer zipped.Close()

	_, err = io.Copy(zipped, original)
	return err
}

func zipConfigCheck(tempDir, hostname string) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetConfigCheck(writer, true) //nolint:errcheck
	writer.Flush()

	return writeConfigCheck(tempDir, hostname, b.Bytes())
}

func writeConfigCheck(tempDir, hostname string, data []byte) error {
	f := filepath.Join(tempDir, hostname, "config-check.log")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(data)
	return err
}

// Used for testing mock HTTP server
var taggerListURL string

func zipTaggerList(tempDir, hostname string) error {
	f := filepath.Join(tempDir, hostname, "tagger-list.json")
	err := ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}

	if taggerListURL == "" {
		taggerListURL = fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}

	c := apiutil.GetClient(false) // FIX: get certificates right then make this true

	r, err := apiutil.DoGet(c, taggerListURL)
	if err != nil {
		return err
	}

	// Pretty print JSON output
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	err = json.Indent(&b, r, "", "\t")
	if err != nil {
		_, err = w.Write(r)
		return err
	}
	writer.Flush()

	_, err = w.Write(b.Bytes())
	return err
}

func zipHealth(tempDir, hostname string) error {
	s := health.GetReady()
	sort.Strings(s.Healthy)
	sort.Strings(s.Unhealthy)

	yamlValue, err := yaml.Marshal(s)
	if err != nil {
		return err
	}

	f := filepath.Join(tempDir, hostname, "health.yaml")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = w.Write(yamlValue)
	return err
}

func zipInstallInfo(tempDir, hostname string) error {
	originalPath := filepath.Join(config.FileUsedDir(), "install_info")
	original, err := os.Open(originalPath)
	if err != nil {
		return err
	}
	defer original.Close()

	zippedPath := filepath.Join(tempDir, hostname, "install_info")
	err = ensureParentDirsExist(zippedPath)
	if err != nil {
		return err
	}

	zipped, err := os.OpenFile(zippedPath, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer zipped.Close()

	_, err = io.Copy(zipped, original)
	return err
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

	f := filepath.Join(tempDir, hostname, filename)
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	w, err := newRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = io.Copy(w, resp.Body)

	return err
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
				f := filepath.Join(tempDir, hostname, "etc", "confd", prefix, baseName)
				err := ensureParentDirsExist(f)
				if err != nil {
					return err
				}

				w, err := newRedactingWriter(f, os.ModePerm, true)
				if err != nil {
					return err
				}
				defer w.Close()

				if _, err = w.WriteFromFile(src); err != nil {
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

func newRedactingWriter(f string, p os.FileMode, buffered bool) (*RedactingWriter, error) {
	w, err := NewRedactingWriter(f, os.ModePerm, true)
	if err != nil {
		return nil, err
	}

	// The original RedactingWriter use the log/strip.go implementation
	// to scrub some credentials.
	// It doesn't deal with api keys of other services, for example powerDNS
	// which has an "api_key" field in its YAML configuration.
	// We add this replacer to scrub even those credentials.
	w.RegisterReplacer(otherAPIKeysReplacer)
	return w, nil
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
	fileName := strings.Join([]string{"datadog", "agent", timeString}, "-")
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
		f := filepath.Join(tempDir, hostname, "etc", filepath.Base(filePath))
		err := ensureParentDirsExist(f)
		if err != nil {
			return err
		}

		w, err := newRedactingWriter(f, os.ModePerm, true)
		if err != nil {
			return err
		}
		defer w.Close()

		_, err = w.WriteFromFile(filePath)
		if err != nil {
			return err
		}

		if permsInfos != nil {
			permsInfos.add(filePath)
		}
	}
	return err
}

// getSystemProbePath would take the path to datadog.yaml and replace the file name with system-probe.yaml
func getSystemProbePath(ddCfgFilePath string) string {
	path := filepath.Dir(ddCfgFilePath)
	return filepath.Join(path, "system-probe.yaml")
}
