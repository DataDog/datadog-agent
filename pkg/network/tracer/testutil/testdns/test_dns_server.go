// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package testdns contains a DNS server for use in testing
package testdns

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

var globalTCPError error
var globalUDPError error
var serverOnce sync.Once

const localhostAddr = "127.0.0.1"

// GetServerIP returns the IP address of the test DNS server. The test DNS server returns canned responses for several
// known domains that are used in integration tests.
//
// see server#start to see which domains are handled.
func GetServerIP(t *testing.T) net.IP {
	t.Helper()
	var srv *server
	serverOnce.Do(func() {
		srv = newServer()
		globalTCPError = srv.Start("tcp")
		globalUDPError = srv.Start("udp")
	})
	require.NoError(t, globalTCPError, "error starting local TCP DNS server")
	require.NoError(t, globalUDPError, "error starting local UDP DNS server")
	return net.ParseIP(localhostAddr)
}

type server struct{}

func newServer() *server {
	return &server{}
}

func (s *server) Start(transport string) error {
	started := make(chan struct{}, 1)
	errChan := make(chan error, 1)
	address := localhostAddr + ":53"
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
		err := srv.ListenAndServe()
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case <-started:
		return nil
	case err := <-errChan:
		return err
	}
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

// SendDNSQueriesOnPort makes a DNS query for every domain provided, on the given serverIP, port, and protocol.
func SendDNSQueriesOnPort(t *testing.T, domains []string, serverIP net.IP, port string, protocol string) (string, int, []*dns.Msg, error) {
	t.Helper()
	dnsClient := dns.Client{Net: protocol, Timeout: 3 * time.Second}
	dnsHost := net.JoinHostPort(serverIP.String(), port)
	conn, err := dnsClient.Dial(dnsHost)
	if err != nil {
		return "", 0, nil, err
	}

	var clientPort int
	var clientIP string
	if protocol == "tcp" {
		clientPort = conn.Conn.(*net.TCPConn).LocalAddr().(*net.TCPAddr).Port
		clientIP = conn.Conn.(*net.TCPConn).LocalAddr().(*net.TCPAddr).IP.String()
	} else { // UDP
		clientPort = conn.Conn.(*net.UDPConn).LocalAddr().(*net.UDPAddr).Port
		clientIP = conn.Conn.(*net.UDPConn).LocalAddr().(*net.UDPAddr).IP.String()
	}
	var reps []*dns.Msg
	msg := new(dns.Msg)
	msg.RecursionDesired = true
	for _, domain := range domains {
		msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
		rep, _, _err := dnsClient.ExchangeWithConn(msg, conn)
		if _err != nil {
			err = multierror.Append(err, fmt.Errorf("failed sending dns query for domain %s to server %s: %w", domain, serverIP, _err))
		}
		reps = append(reps, rep)
	}

	_ = conn.Close()

	return clientIP, clientPort, reps, err
}

// SendDNSQueriesAndCheckError is a simple helper that requires no errors to be present when calling SendDNSQueries
func SendDNSQueriesAndCheckError(
	t *testing.T,
	domains []string,
	serverIP net.IP,
	protocol string,
) (string, int, []*dns.Msg) {
	t.Helper()
	ip, port, resp, err := SendDNSQueries(t, domains, serverIP, protocol)
	require.NoError(t, err)
	return ip, port, resp
}

// SendDNSQueries is a simple helper that calls SendDNSQueriesOnPort with port 53
func SendDNSQueries(
	t *testing.T,
	domains []string,
	serverIP net.IP,
	protocol string,
) (string, int, []*dns.Msg, error) {
	t.Helper()
	return SendDNSQueriesOnPort(t, domains, serverIP, "53", protocol)
}
