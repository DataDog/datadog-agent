// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util provides various functions
package util

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// CopyFile atomically copies file path `src“ to file path `dst`.
func CopyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	perm := fi.Mode()

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, err = io.Copy(tmp, in)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	err = tmp.Close()
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Chmod(tmpName, perm)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Rename(tmpName, dst)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

// CopyFileAll calls CopyFile, but will create necessary directories for  `dst`.
func CopyFileAll(src, dst string) error {
	err := EnsureParentDirsExist(dst)
	if err != nil {
		return err
	}

	return CopyFile(src, dst)
}

// CopyDir copies directory recursively
func CopyDir(src, dst string) error {
	var (
		err     error
		fds     []os.DirEntry
		srcinfo os.FileInfo
	)

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = os.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		s := path.Join(src, fd.Name())
		d := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = CopyDir(s, d)
		} else {
			err = CopyFile(s, d)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// EnsureParentDirsExist makes a path immediately available for
// writing by creating the necessary parent directories.
func EnsureParentDirsExist(p string) error {
	err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

// HTTPHeaders returns a http headers including various basic information (User-Agent, Content-Type...).
func HTTPHeaders() map[string]string {
	av, _ := version.Agent()
	return map[string]string{
		"User-Agent":   fmt.Sprintf("Datadog Agent/%s", av.GetNumber()),
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
}

// GetJSONSerializableMap returns a JSON serializable map from a raw map
func GetJSONSerializableMap(m interface{}) interface{} {
	switch x := m.(type) {
	// unbelievably I cannot collapse this into the next (identical) case
	case map[interface{}]interface{}:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case integration.RawMap:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case integration.JSONMap:
		j := integration.JSONMap{}
		for k, v := range x {
			j[k] = GetJSONSerializableMap(v)
		}
		return j
	case []interface{}:
		j := make([]interface{}, len(x))

		for i, v := range x {
			j[i] = GetJSONSerializableMap(v)
		}
		return j
	}
	return m

}

// GetGoRoutinesDump returns the stack trace of every Go routine of a running Agent.
func GetGoRoutinesDump() (string, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return "", err
	}

	pprofURL := fmt.Sprintf("http://%v:%s/debug/pprof/goroutine?debug=2",
		ipcAddress, config.Datadog.GetString("expvar_port"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := http.Client{}
	req, err := http.NewRequest(http.MethodGet, pprofURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
