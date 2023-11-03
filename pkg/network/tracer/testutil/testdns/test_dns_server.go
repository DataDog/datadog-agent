// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package testdns contains a DNS server for use in testing
package testdns

import (
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var globalServer *server
var globalServerError error
var serverOnce sync.Once
var non53ServerOnce sync.Once

const localhostAddr = "127.0.0.1"

func GetServerIPPort53(t *testing.T) net.IP {
	return GetServerIP(t, 53)
}

// GetServerIP returns the IP address of the test DNS server. The test DNS server returns canned responses for several
// known domains that are used in integration tests.
//
// see server#start to see which domains are handled.
func GetServerIP(t *testing.T, port int) net.IP {
	if port != 53 {
		non53ServerOnce.Do(func() {
			globalServer.Start("udp", port)
		})
	}
	serverOnce.Do(func() {
		globalServer, globalServerError = newServer()
		globalServer.Start("tcp", port)
		globalServer.Start("udp", port)
	})

	require.NoError(t, globalServerError)
	return net.ParseIP(localhostAddr)
}

type server struct{}

func newServer() (*server, error) {
	return &server{}, nil
}

func (s *server) Start(transport string, port int) {
	started := make(chan struct{}, 1)
	address := localhostAddr + ":" + strconv.Itoa(port)
	srv := dns.Server{
		Addr: address,
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
			case "nestedcname.com.":
				resp := &dns.Msg{}
				resp.SetReply(msg)
				top := new(dns.CNAME)
				top.Hdr = dns.RR_Header{Name: "nestedcname.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600}
				top.Target = "www.nestedcname.com."
				nested := new(dns.CNAME)
				nested.Hdr = dns.RR_Header{Name: "www.nestedcname.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600}
				nested.Target = "www2.nestedcname.com."
				ip := new(dns.A)
				ip.Hdr = dns.RR_Header{Name: "www2.nestedcname.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600}
				ip.A = net.ParseIP(localhostAddr)

				resp.Answer = append(resp.Answer, top, nested, ip)
				resp.SetRcode(msg, dns.RcodeSuccess)
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
