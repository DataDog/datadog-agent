// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	emptyPodList = `{"kind":"PodList","apiVersion":"v1","metadata":{},"items":null}
`
	tokenPath    = "testdata/sa-token"
	certAuthPath = "testdata/sa-cacert"
	apiServerUrl = "http://127.0.0.1:8080/api/v1/namespaces/default/secrets"
	saSecret     = "kubernetes.io/service-account-token"
)

type SecretList struct {
	Items []Items `json:"items"`
}

type Items struct {
	Data Data   `json:"data"`
	Type string `json:"type"`
}

type Data struct {
	CaCrt string `json:"ca.crt"`
	Token string `json:"token"`
}

func createCaToken() error {
	c := &http.Client{}
	resp, err := c.Get(apiServerUrl)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	s := &SecretList{}
	err = json.Unmarshal(b, s)
	if err != nil {
		return err
	}
	for _, item := range s.Items {
		if item.Type != saSecret {
			continue
		}
		token, err := base64.StdEncoding.DecodeString(item.Data.Token)
		if err != nil {
			continue
		}
		certificateAuth, err := base64.StdEncoding.DecodeString(item.Data.CaCrt)
		if err != nil {
			continue
		}
		err = ioutil.WriteFile(tokenPath, token, 0600)
		if err != nil {
			return err
		}
		return ioutil.WriteFile(certAuthPath, certificateAuth, 0600)
	}
	return fmt.Errorf("cannot find valid %q token/cacrt in len(%d)", saSecret, len(s.Items))
}

// initOpenKubelet create a standalone kubelet open to http and https calls
func initOpenKubelet() (*utils.ComposeConf, error) {
	networkMode, err := utils.GetNetworkMode()
	if err != nil {
		return nil, err
	}

	compose := &utils.ComposeConf{
		ProjectName: "open_kubelet_kubeutil",
		FilePath:    "testdata/open-kubelet-compose.yaml",
		Variables:   map[string]string{"network_mode": networkMode},
	}
	return compose, nil
}

// initSecureKubelet create an etcd, kube-apiserver and kubelet to open https authNZ calls
func initSecureKubelet(auth bool) (*utils.ComposeConf, *utils.CertificatesConfig, error) {
	networkMode, err := utils.GetNetworkMode()
	if err != nil {
		return nil, nil, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	kubeConfigPath := path.Join(cwd, "testdata/kubeconfig.json")
	_, err = os.Stat(kubeConfigPath)
	if err != nil {
		return nil, nil, err
	}

	certsConfig := &utils.CertificatesConfig{
		Hosts:        "127.0.0.1",
		ValidFor:     time.Duration(24 * time.Hour),
		RsaBits:      1024,
		EcdsaCurve:   "",
		CertFilePath: path.Join(cwd, "testdata/cert.pem"),
		KeyFilePath:  path.Join(cwd, "testdata/key.pem"),
	}
	err = utils.GenerateCertificates(certsConfig)
	if err != nil {
		return nil, nil, err
	}

	projectName := "secure_kubelet_kubeutil"
	composeFile := "secure-kubelet-compose.yaml"
	if auth == true {
		projectName = fmt.Sprintf("auth%s", projectName)
		composeFile = fmt.Sprintf("auth-%s", composeFile)
	}

	compose := &utils.ComposeConf{
		ProjectName: projectName,
		FilePath:    fmt.Sprintf("testdata/%s", composeFile),
		Variables: map[string]string{
			"network_mode":    networkMode,
			"kubeconfig_path": kubeConfigPath,
			"certpem_path":    certsConfig.CertFilePath,
			"keypem_path":     certsConfig.KeyFilePath,
		},
	}
	return compose, certsConfig, nil
}
