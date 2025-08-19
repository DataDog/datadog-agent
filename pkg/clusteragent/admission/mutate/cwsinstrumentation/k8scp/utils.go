// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8scp

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
)

func makeTar(src, dest string, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	stat, err := os.Stat(src)
	if err != nil {
		_ = tarWriter.Close()
		return fmt.Errorf("could not stat %s: %s", src, err)
	}

	// case regular file or other file type like pipe
	hdr, err := tar.FileInfoHeader(stat, src)
	if err != nil {
		_ = tarWriter.Close()
		return err
	}
	hdr.Name = path.Base(dest)

	if err := tarWriter.WriteHeader(hdr); err != nil {
		_ = tarWriter.Close()
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		_ = tarWriter.Close()
		return err
	}

	if _, err := io.Copy(tarWriter, f); err != nil {
		_ = tarWriter.Close()
		_ = f.Close()
		return err
	}

	if err = f.Close(); err != nil {
		_ = tarWriter.Close()
		return fmt.Errorf("could not close file %s: %s", src, err)
	}

	if err = tarWriter.Close(); err != nil {
		return fmt.Errorf("could not close tar writer: %s", err)
	}

	return nil
}
