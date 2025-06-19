// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusteragent contains the logic to create the cluster agent flare archive.
package clusteragent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/comp/core/status"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare"
	flarecommon "github.com/DataDog/datadog-agent/pkg/flare/common"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProfileData maps (pprof) profile names to the profile data
type ProfileData map[string][]byte

// CreateDCAArchive packages up the files
func CreateDCAArchive(local bool, distPath, logFilePath string, pdata ProfileData, statusComponent status.Component, diagnose diagnose.Component, ipc ipc.Component) (string, error) {
	fb, err := flarehelpers.NewFlareBuilder(local, flaretypes.FlareArgs{})
	if err != nil {
		return "", err
	}

	confSearchPaths := map[string]string{
		"":     pkgconfigsetup.Datadog().GetString("confd_path"),
		"dist": filepath.Join(distPath, "conf.d"),
	}

	createDCAArchive(fb, confSearchPaths, logFilePath, pdata, statusComponent, diagnose, ipc)
	return fb.Save()
}

func createDCAArchive(fb flaretypes.FlareBuilder, confSearchPaths map[string]string, logFilePath string, pdata ProfileData, statusComponent status.Component, diagnose diagnose.Component, ipc ipc.Component) {
	// If the request against the API does not go through we don't collect the status log.
	if fb.IsLocal() {
		fb.AddFile("local", nil) //nolint:errcheck
	} else {
		// The Status will be unavailable unless the agent is running.
		// Only zip it up if the agent is running
		err := fb.AddFileFromFunc("cluster-agent-status.log", func() ([]byte, error) { return statusComponent.GetStatus("text", true) }) //nolint:errcheck
		if err != nil {
			log.Errorf("Error getting the status of the DCA, %q", err)
			return
		}

	}

	if fb.IsLocal() {
		getLocalClusterAgentDiagnose(fb, diagnose) //nolint:errcheck
	} else {
		getClusterAgentDiagnose(fb, diagnose) //nolint:errcheck
	}

	remote := &flare.RemoteFlareProvider{
		IPC: ipc,
	}

	client := ipc.GetClient()

	flarecommon.GetLogFiles(fb, logFilePath)
	flarecommon.GetConfigFiles(fb, confSearchPaths)
	getClusterAgentConfigCheck(fb, client)   //nolint:errcheck
	flarecommon.GetExpVar(fb)                //nolint:errcheck
	getMetadataMap(fb)                       //nolint:errcheck
	getClusterAgentClusterChecks(fb, client) //nolint:errcheck

	fb.AddFileFromFunc("agent-daemonset.yaml", getAgentDaemonSet)                                                                    //nolint:errcheck
	fb.AddFileFromFunc("cluster-agent-deployment.yaml", getClusterAgentDeployment)                                                   //nolint:errcheck
	fb.AddFileFromFunc("helm-values.yaml", getHelmValues)                                                                            //nolint:errcheck
	fb.AddFileFromFunc("datadog-agent-cr.yaml", getDatadogAgentManifest)                                                             //nolint:errcheck
	fb.AddFileFromFunc("envvars.log", flarecommon.GetEnvVars)                                                                        //nolint:errcheck
	fb.AddFileFromFunc("telemetry.log", QueryDCAMetrics)                                                                             //nolint:errcheck
	fb.AddFileFromFunc("autoscaler-list.json", func() ([]byte, error) { return getDCAAutoscalerList(remote) })                       //nolint:errcheck
	fb.AddFileFromFunc("local-autoscaling-check.json", func() ([]byte, error) { return getDCALocalAutoscalingWorkloadList(remote) }) //nolint:errcheck
	fb.AddFileFromFunc("tagger-list.json", func() ([]byte, error) { return getDCATaggerList(remote) })                               //nolint:errcheck
	fb.AddFileFromFunc("workload-list.log", func() ([]byte, error) { return getDCAWorkloadList(remote) })                            //nolint:errcheck
	fb.AddFileFromFunc("cluster-agent-metadata.json", func() ([]byte, error) { return getClusterAgentMetadataPayload(client) })      //nolint:errcheck
	getPerformanceProfileDCA(fb, pdata)

	if pkgconfigsetup.Datadog().GetBool("external_metrics_provider.enabled") {
		getHPAStatus(fb) //nolint:errcheck
	}
}

// QueryDCAMetrics gets the metrics payload exposed by the cluster agent
func QueryDCAMetrics() ([]byte, error) {
	r, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", pkgconfigsetup.Datadog().GetInt("metrics_port")))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func getMetadataMap(fb flaretypes.FlareBuilder) error {
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

	str, err := render.FormatMetadataMapCLI(metaBytes)
	if err != nil {
		return log.Errorf("Error while rendering the cluster level metadata: %q", err)
	}

	return fb.AddFile("cluster-agent-metadatamapper.log", []byte(str))
}

func getClusterAgentClusterChecks(fb flaretypes.FlareBuilder, client ipc.HTTPClient) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterChecks(writer, "", client) //nolint:errcheck
	writer.Flush()

	return fb.AddFile("clusterchecks.log", b.Bytes())
}

func getHPAStatus(fb flaretypes.FlareBuilder) error {
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

	str, err := render.FormatHPAStatus(statsBytes)
	if err != nil {
		return log.Errorf("Could not collect custommetricsprovider.log: %s", err)
	}

	return fb.AddFile("custommetricsprovider.log", []byte(str))
}

func getClusterAgentConfigCheck(fb flaretypes.FlareBuilder, client ipc.HTTPClient) error {
	var b bytes.Buffer

	writer := bufio.NewWriter(&b)
	GetClusterAgentConfigCheck(writer, true, client) //nolint:errcheck
	writer.Flush()

	return fb.AddFile("config-check.log", b.Bytes())
}

// GetClusterAgentConfigCheck gets config check from the server for cluster agent
func GetClusterAgentConfigCheck(w io.Writer, withDebug bool, client ipc.HTTPClient) error {
	targetURL := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("localhost:%v", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port")),
		Path:   "config-check",
	}

	r, err := client.Get(targetURL.String(), ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while checking config: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	cr := integration.ConfigCheckResponse{}
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return err
	}

	flare.PrintConfigCheck(w, cr, withDebug)

	return nil
}

func getClusterAgentDiagnose(fb flaretypes.FlareBuilder, diagnose diagnose.Component) error {
	bytes, err := GetClusterAgentDiagnose(diagnose)
	if err != nil {
		return err
	}

	return fb.AddFile("diagnose.log", bytes)
}

func getLocalClusterAgentDiagnose(fb flaretypes.FlareBuilder, diagnose diagnose.Component) error {
	bytes, err := GetLocalClusterAgentDiagnose(diagnose)
	if err != nil {
		return err
	}

	return fb.AddFile("diagnose.log", bytes)
}

func getDCAAutoscalerList(remote *flare.RemoteFlareProvider) ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	autoscalerListURL := fmt.Sprintf("https://%v:%v/autoscaler-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))

	r, err := remote.IPC.GetClient().Get(autoscalerListURL, ipchttp.WithCloseConnection)
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

func getDCALocalAutoscalingWorkloadList(remote *flare.RemoteFlareProvider) ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}
	localAutoscalingWorkloadListURL := fmt.Sprintf("https://%v:%v/local-autoscaling-check", ipcAddress, pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))
	r, err := remote.IPC.GetClient().Get(localAutoscalingWorkloadListURL, ipchttp.WithCloseConnection)
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

func getDCATaggerList(remote *flare.RemoteFlareProvider) ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	taggerListURL := fmt.Sprintf("https://%v:%v/tagger-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))

	return remote.GetTaggerList(taggerListURL)
}

func getDCAWorkloadList(remote *flare.RemoteFlareProvider) ([]byte, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	return remote.GetWorkloadList(fmt.Sprintf("https://%v:%v/workload-list?verbose=true", ipcAddress, pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port")))
}

func getClusterAgentMetadataPayload(client ipc.HTTPClient) ([]byte, error) {
	targetURL := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("localhost:%v", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port")),
		Path:   "metadata/cluster-agent",
	}

	r, err := client.Get(targetURL.String(), ipchttp.WithCloseConnection)
	if err != nil {
		if r != nil && string(r) != "" {
			return nil, fmt.Errorf("the agent ran into an error while checking dca metadata: %s", string(r))
		}
		return nil, fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	return r, nil
}

func getPerformanceProfileDCA(fb flaretypes.FlareBuilder, pdata ProfileData) {
	for name, data := range pdata {
		fb.AddFileWithoutScrubbing(filepath.Join("profiles", name), data) //nolint:errcheck
	}
}
