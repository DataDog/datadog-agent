// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package api

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"

	log "github.com/cihub/seelog"
)

// getListener returns a listening connection to a Unix socket
// on non-windows platforms.
func getListener() (net.Listener, error) {
	listener, err := net.Listen("unix", config.Datadog.GetString("cmd_sock"))
	if err != nil {
		up, e := checkIfUp()
		if e != nil {
			log.Info("here it is")
			return listener, err
		}
		if !up {
			e := os.Remove(config.Datadog.GetString("cmd_sock"))
			if e != nil {
				log.Infof("cannot remove cmd_sock: %v", e)
				return listener, e
			}
			listener, e = net.Listen("unix", config.Datadog.GetString("cmd_sock"))
			if e != nil {
				return listener, e
			}
		}
	}
	return listener, nil
}

// HTTP doesn't need anything from TCP so we can use a Unix socket to dial
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	sockname := "/tmp/" + addr
	sockname = strings.Split(sockname, ":")[0]

	return net.Dial("unix", sockname)
}

func checkIfUp() (bool, error) {
	c := GetClient()
	url := "http://" + strings.SplitAfter(config.Datadog.GetString("cmd_sock"), "tmp/")[1] + "/agent/version"
	r, e := c.Get(url)
	if e != nil {
		if strings.Contains(e.Error(), "connection refused") {
			return false, nil
		}
		return false, e
	}
	body, e := ioutil.ReadAll(r.Body)
	r.Body.Close()
	// if it returns an error, it's probably not up
	if e != nil {
		return false, e
	}
	// if it returns a status code, it's up
	if r.StatusCode >= 1 {
		return true, fmt.Errorf("Status code too high")
	}
	// if it doesn't return a body, it's probably not up
	if string(body) == "" {
		return false, e
	}
	return true, nil
}

// GetClient is a convenience function returning an http
// client suitable to use a unix socket transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
