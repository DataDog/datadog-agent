// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package utils has common utility methods that components can use for structuring http responses of their endpoints
package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	grpccontext "github.com/DataDog/datadog-agent/pkg/util/grpc/context"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetConnection returns the connection for the request
func GetConnection(r *http.Request) net.Conn {
	return r.Context().Value(grpccontext.ConnContextKey).(net.Conn)
}

// StreamRequest sends a request to the given url for the given duration
func StreamRequest(url string, body []byte, duration time.Duration, onChunk func([]byte)) error {
	c := apiutil.GetClient(false)
	if duration != 0 {
		c.Timeout = duration
	}
	// Set session token
	e := apiutil.SetAuthToken(pkgconfig.Datadog())
	if e != nil {
		return e
	}

	e = apiutil.DoPostChunked(c, url, "application/json", bytes.NewBuffer(body), onChunk)

	if e == io.EOF {
		return nil
	}
	if e != nil {
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the logs and contact support if you continue having issues. \n", e)
	}
	return e
}

// OpenFileForWriting opens a file for writing
func OpenFileForWriting(filePath string) (*os.File, *bufio.Writer, error) {
	log.Infof("opening file %s for writing", filePath)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening file %s: %v", filePath, err)
	}
	bufWriter := bufio.NewWriter(f) // default 4096 bytes buffer
	return f, bufWriter, nil
}

// CheckDirExists checks if the directory for the given path exists, if not then create it.
func CheckDirExists(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}
