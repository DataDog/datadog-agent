// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ProfileDataDCA map[string][]byte

// CreatePerformanceProfile adds a set of heap and CPU profiles into target, using cpusec as the CPU
// profile duration, debugURL as the target URL for fetching the profiles and prefix as a prefix for
// naming them inside target.
//
// It is accepted to pass a nil target.
func CreatePerformanceProfileDCA(prefix, debugURL string, cpusec int, target *ProfileDataDCA) error {

	c := apiutil.GetClient(false)
	if *target == nil {
		*target = make(ProfileDataDCA)
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

// CreateDCAArchive packages up the files
func CreateDCAArchive(local bool, distPath, logFilePath string, pdata ProfileDataDCA) (string, error) {
	fb, err := flarehelpers.NewFlareBuilder()
	if err != nil {
		return "", err
	}

	confSearchPaths := SearchPaths{
		"":     config.Datadog.GetString("confd_path"),
		"dist": filepath.Join(distPath, "conf.d"),
	}

	createDCAArchive(fb, local, confSearchPaths, logFilePath, pdata)
	return fb.Save()
}

func createDCAArchive(fb flarehelpers.FlareBuilder, local bool, confSearchPaths SearchPaths, logFilePath string, pdata ProfileDataDCA) {
	// If the request against the API does not go through we don't collect the status log.
	if local {
		fb.AddFile("local", []byte(""))
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err := fb.AddFileFromFunc("cluster-agent-status.log", status.GetAndFormatDCAStatus)
		if err != nil {
			log.Errorf("Error getting the status of the DCA, %q", err)
			return
		}
	}

	getLogFiles(fb, logFilePath)
	getConfigFiles(fb, confSearchPaths)
	getClusterAgentConfigCheck(fb)   //nolint:errcheck
	getExpVar(fb)                    //nolint:errcheck
	getMetadataMap(fb)               //nolint:errcheck
	getClusterAgentClusterChecks(fb) //nolint:errcheck
	getClusterAgentDiagnose(fb)      //nolint:errcheck
	fb.AddFileFromFunc("agent-daemonset.yaml", getAgentDaemonSet)
	fb.AddFileFromFunc("cluster-agent-deployment.yaml", getClusterAgentDeployment)
	fb.AddFileFromFunc("helm-values.yaml", getHelmValues)
	fb.AddFileFromFunc("envvars.log", getEnvVars)
	fb.AddFileFromFunc("telemetry.log", QueryDCAMetrics)
	fb.AddFileFromFunc("tagger-list.json", getDCATaggerList)
	fb.AddFileFromFunc("workload-list.log", getDCAWorkloadList)

	getPerformanceProfileDCA(fb, pdata)

	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		getHPAStatus(fb) //nolint:errcheck
	}
}

// QueryDCAMetrics gets the metrics payload exposed by the cluster agent
func QueryDCAMetrics() ([]byte, error) {
	r, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", config.Datadog.GetInt("metrics_port")))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return ioutil.ReadAll(r.Body)
}

func getMetadataMap(fb flarehelpers.FlareBuilder) error {
	metaList := apiv1.NewMetadataResponse()
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		metaList.Errors = fmt.Sprintf("Can't create client to query the API Server: %s", err.Error())
	} else {
		// Grab the metadata map for all nodes.
		metaList, err = apiserver.GetMetadataMapBundleOnAllNodes(cl)
		if err != nil {
			log.Infof("Error while collecting the cluster level metadata: %q", err)
		}
	}

	metaBytes, err := json.Marshal(metaList)
	if err != nil {
		return log.Errorf("Error while marshalling the cluster level metadata: %q", err)
	}

	str, err := status.FormatMetadataMapCLI(metaBytes)
	if err != nil {
		return log.Errorf("Error while rendering the cluster level metadata: %q", err)
	}

	return fb.AddFile("cluster-agent-metadatamapper.log", []byte(str))
}

func getClusterAgentClusterChecks(fb flarehelpers.FlareBuilder) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterChecks(writer, "") //nolint:errcheck
	writer.Flush()

	return fb.AddFile("clusterchecks.log", b.Bytes())
}

func getHPAStatus(fb flarehelpers.FlareBuilder) error {
	stats := make(map[string]interface{})
	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		stats["custommetrics"] = map[string]string{"Error": err.Error()}
	} else {
		stats["custommetrics"] = custommetrics.GetStatus(apiCl.Cl)
	}
	statsBytes, err := json.Marshal(stats)
	if err != nil {
		return log.Errorf("Error while marshalling the cluster level metadata: %q", err)
	}

	str, err := status.FormatHPAStatus(statsBytes)
	if err != nil {
		return log.Errorf("Could not collect custommetricsprovider.log: %s", err)
	}

	return fb.AddFile("custommetricsprovider.log", []byte(str))
}

func getClusterAgentConfigCheck(fb flarehelpers.FlareBuilder) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterAgentConfigCheck(writer, true) //nolint:errcheck
	writer.Flush()

	return fb.AddFile("config-check.log", b.Bytes())
}

func getClusterAgentDiagnose(fb flarehelpers.FlareBuilder) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterAgentDiagnose(writer) //nolint:errcheck
	writer.Flush()

	return fb.AddFile("diagnose.log", b.Bytes())
}

func getDCATaggerList() ([]byte, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%v:%v/tagger-list", ipcAddress, config.Datadog.GetInt("cluster_agent.cmd_port"))

	return getTaggerList(taggerListURL)
}

func getDCAWorkloadList() ([]byte, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	return getWorkloadList(fmt.Sprintf("https://%v:%v/workload-list?verbose=true", ipcAddress, config.Datadog.GetInt("cluster_agent.cmd_port")))
}

func getPerformanceProfileDCA(fb flarehelpers.FlareBuilder, pdata ProfileDataDCA) {
	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data)
	}
}
