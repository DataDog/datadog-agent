// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubernetes

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	emptyPodList = `{"kind":"PodList","apiVersion":"v1","metadata":{},"items":null}
`
)

// initInsecureKubelet create a standalone kubelet open to http and https calls
func initInsecureKubelet() (*utils.ComposeConf, error) {
	compose := &utils.ComposeConf{
		ProjectName: "insecure_kubelet",
		FilePath:    "testdata/insecure-kubelet-compose.yaml",
		Variables:   map[string]string{},
	}
	return compose, nil
}

// initSecureKubelet create an etcd, kube-apiserver and kubelet to open https authNZ calls
// auth parameter allows to switch to secure + authenticated setup
func initSecureKubelet() (*utils.ComposeConf, *utils.CertificatesConfig, error) {
	cwd, err := os.Getwd()
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

	projectName := "kubelet"
	composeFile := "secure-kubelet-compose.yaml"

	compose := &utils.ComposeConf{
		ProjectName: projectName,
		FilePath:    fmt.Sprintf("testdata/%s", composeFile),
		Variables: map[string]string{
			"certpem_path": certsConfig.CertFilePath,
			"keypem_path":  certsConfig.KeyFilePath,
		},
		RemoveRebuildImages: true,
	}
	// try to remove any old staling resources, especially images
	// this is because an old image can contain old certificates, key
	// issued from a previous unTearDown build/test
	compose.Stop()
	return compose, certsConfig, nil
}
