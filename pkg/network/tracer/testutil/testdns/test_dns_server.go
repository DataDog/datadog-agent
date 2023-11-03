// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package testdns contains a DNS server for use in testing
package testdns

import (
	"net"
	"sync"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var globalServer *server
var globalServerError error
var serverOnce sync.Once

// GetServerIP returns the IP address of the test DNS server. The test DNS server returns canned responses for several
// known domains that are used in integration tests.
//
// see server#start to see which domains are handled.
func GetServerIP(t *testing.T) net.IP {
	serverOnce.Do(func() {
		globalServer, globalServerError = newServer()
		globalServer.Start("tcp")
		globalServer.Start("udp")
	})

	require.NoError(t, globalServerError)
	return net.ParseIP("127.0.0.1")
}

type server struct{}

func newServer() (*server, error) {
	return &server{}, nil
}

func (s *server) Start(transport string) {
	started := make(chan struct{}, 1)
	srv := dns.Server{
		Addr: "127.0.0.1:53",
		Net:  transport,
		Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, msg *dns.Msg) {
			switch msg.Question[0].Name {
			case "good.com.":
				respond(msg, writer, "good.com. 30 IN A  10.0.0.1")
			case "golang.org.":
				respond(msg, writer, "golang.org. 30 IN A  10.0.0.2")
			case "google.com.":
				respond(msg, writer, "google.com. 30 IN A  10.0.0.3")
			case "acm.org.":
				respond(msg, writer, "acm.org. 30 IN A  10.0.0.4")
			case "nonexistenent.net.com.":
				resp := &dns.Msg{}
				resp.SetReply(msg)
				resp.Rcode = dns.RcodeNameError
				_ = writer.WriteMsg(resp)
			case "missingdomain.com.":
				resp := &dns.Msg{}
				resp.SetReply(msg)
				resp.Rcode = dns.RcodeNameError
				_ = writer.WriteMsg(resp)
			default:
				resp := &dns.Msg{}
				resp.SetReply(msg)
				resp.Rcode = dns.RcodeServerFailure
				_ = writer.WriteMsg(resp)
			}
		}),
		NotifyStartedFunc: func() {
			started <- struct{}{}
		},
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
	<-started
}

func respond(req *dns.Msg, writer dns.ResponseWriter, record string) {
	resp := &dns.Msg{}
	resp.SetReply(req)

	rr, err := dns.NewRR(record)
	if err != nil {
		panic(err)
	}
	resp.Answer = []dns.RR{rr}
	_ = writer.WriteMsg(resp)
}
