// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package npm contains Windows NPM end-to-end tests.
//
// This test reproduces the post-PR-#47005 case where Windows hosts still
// report TCP failure rates over 100%.
//
// Root cause (confirmed by reading ddnpm source at
// DataDog/windows-drivers/ddnpm/ddfilter/flow/):
//
//   - The driver keeps two flow tables: openFlows (read non-destructively
//     every poll) and closedFlows (read-once, removed afterwards, with
//     FLOW_CLOSED_MASK set on the way out).
//   - When a TCP connection terminates, ddnpm updates its connectionStatus
//     in-place inside openFlows, then tries to move it to closedFlows via
//     DecrementRefCountAndMoveFromOpenToClosedTable. That function only
//     succeeds when refcount == 2 (the open table + the calling callout).
//   - If any other WFP callout holds a reference at that moment — e.g. the
//     HTTP/TLS protocol-classification callouts that the driver itself
//     spawns — the move is silently skipped. The flow stays in openFlows
//     with a terminal status like CONN_STAT_EST_RECV_RST.
//   - Every subsequent poll re-reports the same flow. The agent maps
//     connectionStatus → TCPFailures every time but FLOW_CLOSED_MASK is
//     never set, so Last.TCPClosed is 1 only on the first poll (delta) and
//     0 afterwards. TCPFailures is shipped cumulatively → backend rate
//     exceeds 100%.
//
// Earlier versions of this test used simple bytes-over-TCP plus an
// abortive close. That produces zero protocol-classification callouts, so
// when the RST hits, refcount == 2 and the move succeeds immediately:
// observed rate exactly 100% (not the bug). v12 added real HTTP/1.1 traffic
// so the driver's HTTP classifier engages — but classification finishes well
// before the 1s server hold and the RST, so refcount drops back to 2 in
// time. Observed rate <100% (also not the bug), because some loopback
// connections elsewhere on the host close cleanly and dilute the failure
// numerator.
//
// v13 escalates on three axes simultaneously, each justified independently:
//  1. TLS — server wraps the accepted TCP conn with tls.Server and the
//     client uses tls.Client. The TLS classifier engages on the first
//     packets and stays engaged longer than HTTP-over-cleartext does
//     (multiple handshake packets, more state). We deliberately never
//     call tls.Conn.Close() — that would write a close_notify alert and
//     trigger a graceful TCP close. We let the deferred raw *net.TCPConn
//     Close run, with SO_LINGER=0 set up front, so the close becomes a
//     RST regardless of which TLS state we exited from.
//  2. Server hold 1s → 3s. Gives the classifier longer to be engaged when
//     the RST hits.
//  3. Workers 16 → 64. More concurrent flows = more chances for any one
//     flow's termination to coincide with another flow's classification.
//
// The server and client are two separate processes spawned from the same
// Go binary via -mode={server,client}. Distinct PIDs make ddnpm associate
// the loopback halves with different processes — necessary because v5
// suggested ddnpm may dedupe same-process loopback flows (untested but
// cheap to defeat).
//
// This test does NOT use fakeintake. Connection data ships to the real
// Datadog backend via the API key configured in your Pulumi stack.
//
// Filtering caveat with --keep-stack: the e2e framework writes a new
// `tags:` entry to datadog.yaml on each test iteration but does NOT
// restart datadogagent, so the agent keeps shipping data with the very
// first iteration's e2e_test_run tag. To identify a specific run, filter
// by host + a 5-minute time window around the "start (UTC)" / "end (UTC)"
// timestamps printed in the test logs, not by the per-run tag.
package npm

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenwin "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// systemProbeConfigNPM enables NPM on the Windows agent.
const systemProbeConfigNPM = `network_config:
  enabled: true
`

// trafficDuration is how long the Go HTTP client runs. We run the client
// foreground over SSH now (so host.Execute blocks for the duration), so keep
// this short for fast iteration. Bump it once we know the binary works.
const trafficDuration = 30 * time.Second

// workDir on the Windows host. Hosts the Go binary and its logs.
const workDir = `C:\dd-tcp-failure-replay`

// serverPort is the port the embedded HTTP server binds on the host.
const serverPort = 19999

// clientTargetIP is the IP the client dials. We use the host's real VPC IP
// (not loopback) so traffic goes through the real ENI → NDIS → WFP callout
// path. Loopback traffic is synchronous in the Windows kernel and can never
// trigger the concurrent IncrementFdRefcount / flowDeleteNotify race that
// causes the >100% bug. The server binds 0.0.0.0 so any IP on the host works.
const clientTargetIP = "10.1.60.22"

// httpRstServerScript: PowerShell HTTP/1.1 listener bound to 127.0.0.1.
// For each accepted connection:
//   - Reads request bytes until "\r\n\r\n" (end of HTTP headers).
//   - Writes a valid HTTP/1.1 response head with Content-Length: 1000000.
//   - Writes a short partial body so the wire content is unambiguously HTTP.
//   - Sleeps ~1s — gives ddnpm's HTTP classifier time to engage and (we
//     hope) hold an extra refcount on the flow.
//   - SO_LINGER=0 close → TCP RST mid-response.
//
// We deliberately keep this in PowerShell (image: powershell.exe) so the
// client (image: tcp-rst-client.exe) and the server live in different
// processes AND different binaries — defeating any same-image dedupe the
// driver might apply when correlating loopback flows. This matches v4's
// architecture, which produced NPM-visible connections.
const httpRstServerScript = `
$ErrorActionPreference = 'SilentlyContinue'
$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, %[1]d)
$listener.Start(1024)  # large backlog so SYNs queue instead of RST when accept is slow
Write-Host "[http-rst-server] listening on 127.0.0.1:%[1]d"
$pool = [RunspaceFactory]::CreateRunspacePool(1, 64)
$pool.Open()
while ($true) {
  $client = $listener.AcceptTcpClient()
  $ps = [PowerShell]::Create()
  $ps.RunspacePool = $pool
  [void]$ps.AddScript({
    param($c)
    try {
      $c.NoDelay = $true
      $s = $c.GetStream()
      # Read until end-of-headers (\r\n\r\n) or up to 4KB, whichever first.
      $buf = New-Object byte[] 4096
      $total = 0
      while ($total -lt $buf.Length) {
        $n = $s.Read($buf, $total, $buf.Length - $total)
        if ($n -le 0) { break }
        $total += $n
        # Look for end of headers.
        if ($total -ge 4) {
          $found = $false
          for ($i = 0; $i -le $total - 4; $i++) {
            if ($buf[$i] -eq 13 -and $buf[$i+1] -eq 10 -and $buf[$i+2] -eq 13 -and $buf[$i+3] -eq 10) { $found = $true; break }
          }
          if ($found) { break }
        }
      }
      $cr = [char]13
      $lf = [char]10
      $crlf = "$cr$lf"
      $resp = "HTTP/1.1 200 OK" + $crlf + "Server: powershell-http-rst-server" + $crlf + "Content-Type: text/plain" + $crlf + "Content-Length: 1000000" + $crlf + "Connection: close" + $crlf + $crlf + "partial body here, RST incoming..."
      $respBytes = [System.Text.Encoding]::ASCII.GetBytes($resp)
      $s.Write($respBytes, 0, $respBytes.Length)
      $s.Flush()
      Start-Sleep -Seconds 1
      $c.LingerState = [System.Net.Sockets.LingerOption]::new($true, 0)
    } catch {}
    finally { try { $c.Close() } catch {} }
  }).AddArgument($client)
  [void]$ps.BeginInvoke()
}
`

// goReplayClientSource: a single Go program with -mode={server,client} that
// exchanges HTTP/1.1 over TLS and RSTs every connection.
//
// Key invariants (each chosen to defeat a specific failure mode observed in
// earlier versions of this test):
//
//   - SetLinger(0) is set on the raw *net.TCPConn BEFORE any reads/writes.
//     That means any exit path — TLS handshake failure, read/write deadline,
//     short read, deferred Close — RSTs the connection rather than closing
//     it cleanly. v12 set linger inside a defer, which was racy with the
//     write path and produced some non-RST closes.
//   - We never call tls.Conn.Close(). That call writes a TLS close_notify
//     alert (1 encrypted byte) and then closes the underlying conn — a
//     graceful TCP close. We want a RST, so we let the deferred
//     tcp.Close() (on the raw *net.TCPConn) run and skip the TLS layer's
//     shutdown.
//   - Server and client are separate processes via Start-Process / & '%s'.
//     Distinct PIDs mean ddnpm associates the loopback halves with
//     different processes.
//
// Logs go through a -log <path> flag using os.OpenFile so we don't lose
// stdout to Windows's Start-Process redirection buffering. -stdout=true
// additionally tees to stdout for foreground execution where host.Execute
// captures stdout directly.
//
// Flags:
//
//	-mode={server,client}    required
//	-bind=0.0.0.0:19999      server only
//	-target=127.0.0.1:19999  client only
//	-tls=true                wrap with TLS on both sides
//	-hold=3s                 server hold duration after writing response
//	-workers=64              client worker goroutines
//	-duration=30s            client run duration
//	-log=<path>              log file (in addition to stdout if -stdout)
//	-stdout                  also tee log lines to stdout
const goReplayClientSource = `package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	mrand "math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	// Immediate stdout proof-of-life — happens BEFORE flag parsing or any
	// other logic. If we don't see this in captured stdout, the binary
	// isn't running at all (AV, missing DLL, etc.).
	fmt.Printf("[boot] tcp-rst-replay started pid=%d args=%v\n", os.Getpid(), os.Args)

	mode := flag.String("mode", "", "server or client")
	bindAddr := flag.String("bind", "0.0.0.0:19999", "server bind address (mode=server)")
	target := flag.String("target", "127.0.0.1:19999", "target host:port (mode=client)")
	useTLS := flag.Bool("tls", false, "wrap connection with TLS on both sides")
	workers := flag.Int("workers", 64, "client workers (mode=client)")
	durationStr := flag.String("duration", "5m", "test duration (mode=client)")
	holdStr := flag.String("hold", "1s", "server hold duration after writing response (mode=server)")
	logPath := flag.String("log", "", "log file path (optional)")
	stdoutLog := flag.Bool("stdout", false, "if true, also tee log lines to stdout")
	flag.Parse()
	fmt.Printf("[boot] parsed flags: mode=%q bind=%q target=%q tls=%v workers=%d duration=%s hold=%s log=%q stdout=%v\n",
		*mode, *bindAddr, *target, *useTLS, *workers, *durationStr, *holdStr, *logPath, *stdoutLog)

	var writers []io.Writer
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open log:", err)
			os.Exit(2)
		}
		defer f.Close()
		writers = append(writers, f)
		fmt.Printf("[boot] log file opened: %s\n", *logPath)
	}
	if *stdoutLog || len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}
	log.SetOutput(io.MultiWriter(writers...))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	hold, err := time.ParseDuration(*holdStr)
	if err != nil {
		log.Fatalf("bad hold duration: %v", err)
	}

	switch *mode {
	case "server":
		runServer(*bindAddr, *useTLS, hold)
	case "client":
		runClientMain(*target, *useTLS, *workers, *durationStr)
	default:
		log.Fatalf("--mode must be server or client (got %q)", *mode)
	}
}

func generateSelfSignedCert() tls.Certificate {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("rsa.GenerateKey: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		log.Fatalf("rand.Int: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "tcp-rst-replay"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("x509.CreateCertificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

func runServer(bindAddr string, useTLS bool, hold time.Duration) {
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", bindAddr, err)
	}
	log.Printf("[server] listening on %s tls=%v hold=%s pid=%d", bindAddr, useTLS, hold, os.Getpid())

	var tlsCfg *tls.Config
	if useTLS {
		tlsCfg = &tls.Config{Certificates: []tls.Certificate{generateSelfSignedCert()}}
	}

	var handled int64
	go func() {
		t := time.NewTicker(15 * time.Second)
		for range t.C {
			log.Printf("[server] handled=%d", atomic.LoadInt64(&handled))
		}
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("[server] accept err: %v", err)
			continue
		}
		go func(c net.Conn) {
			handleServerConn(c, tlsCfg, hold)
			atomic.AddInt64(&handled, 1)
		}(c)
	}
}

func handleServerConn(rawConn net.Conn, tlsCfg *tls.Config, _ time.Duration) {
	tcp := rawConn.(*net.TCPConn)
	// SetLinger(0) BEFORE any I/O so every exit path RSTs the socket.
	_ = tcp.SetLinger(0)
	defer tcp.Close()

	var conn net.Conn = tcp
	if tlsCfg != nil {
		tlsConn := tls.Server(tcp, tlsCfg)
		_ = tcp.SetDeadline(time.Now().Add(5 * time.Second))
		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			return
		}
		_ = tcp.SetDeadline(time.Time{})
		conn = tlsConn
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return
		}
		total += n
		if total >= 4 && containsHeaderEnd(buf[:total]) {
			break
		}
	}

	// Stream a 256 KB body as fast as possible, then return immediately.
	// The deferred tcp.Close() with linger=0 fires RST while the outbound
	// data segments are still flowing through NDIS and WFP's transport
	// callout — that concurrent IncrementFdRefcount in the transport
	// callout is what races with the 4th flowDeleteNotify CAS and triggers
	// the >100% bug. No sleep: maximum overlap between data DPCs and RST.
	resp := "HTTP/1.1 200 OK\r\n" +
		"Server: tcp-rst-replay\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"Content-Length: 262144\r\n" +
		"Connection: close\r\n" +
		"\r\n"
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(resp)); err != nil {
		return
	}
	body := make([]byte, 262144)
	conn.Write(body) //nolint:errcheck // RST fires on return; error expected
	// Return → deferred tcp.Close() sends RST.
	// Deliberately do NOT call tls.Conn.Close() — that would send a TLS
	// close_notify alert and trigger a graceful TCP close instead of a RST.
}

func containsHeaderEnd(b []byte) bool {
	for i := 0; i+3 < len(b); i++ {
		if b[i] == '\r' && b[i+1] == '\n' && b[i+2] == '\r' && b[i+3] == '\n' {
			return true
		}
	}
	return false
}

func runClientMain(target string, useTLS bool, workers int, durationStr string) {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Fatalf("bad duration: %v", err)
	}
	log.Printf("[client] target=%s tls=%v workers=%d duration=%s pid=%d", target, useTLS, workers, duration, os.Getpid())

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var attempts, dialFails, handshakeFails, readBytes int64

	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				log.Printf("[client] attempts=%d dial_fails=%d handshake_fails=%d read_bytes=%d",
					atomic.LoadInt64(&attempts),
					atomic.LoadInt64(&dialFails),
					atomic.LoadInt64(&handshakeFails),
					atomic.LoadInt64(&readBytes))
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := mrand.New(mrand.NewSource(time.Now().UnixNano() + int64(id)))
			for ctx.Err() == nil {
				atomic.AddInt64(&attempts, 1)
				n, dialOK, hsOK := doOne(target, useTLS, id, rng)
				if !dialOK {
					atomic.AddInt64(&dialFails, 1)
				} else if !hsOK {
					atomic.AddInt64(&handshakeFails, 1)
				}
				atomic.AddInt64(&readBytes, n)
			}
		}(i)
	}
	wg.Wait()
	log.Printf("[client] FINAL attempts=%d dial_fails=%d handshake_fails=%d read_bytes=%d",
		atomic.LoadInt64(&attempts),
		atomic.LoadInt64(&dialFails),
		atomic.LoadInt64(&handshakeFails),
		atomic.LoadInt64(&readBytes))
}

// doOne returns (readBytes, dialOK, handshakeOK). A handshake failure still
// counts as dialOK=true because the TCP layer connected.
func doOne(target string, useTLS bool, id int, rng *mrand.Rand) (int64, bool, bool) {
	rawConn, err := net.DialTimeout("tcp", target, 3*time.Second)
	if err != nil {
		return 0, false, false
	}
	tcp := rawConn.(*net.TCPConn)
	_ = tcp.SetLinger(0)
	defer tcp.Close()

	var conn net.Conn = tcp
	if useTLS {
		tlsConn := tls.Client(tcp, &tls.Config{InsecureSkipVerify: true, ServerName: "tcp-rst-replay"})
		_ = tcp.SetDeadline(time.Now().Add(3 * time.Second))
		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			return 0, true, false
		}
		_ = tcp.SetDeadline(time.Time{})
		conn = tlsConn
	}

	req := fmt.Sprintf(
		"GET /replay?worker=%d&n=%d HTTP/1.1\r\n"+
			"Host: tcp-rst-replay\r\n"+
			"User-Agent: tcp-rst-replay/1\r\n"+
			"Accept: */*\r\n"+
			"Connection: close\r\n"+
			"\r\n",
		id, rng.Int63())
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, true, true
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var nread int64
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		nread += int64(n)
		if err != nil {
			break
		}
	}
	return nread, true, true
}
`

type windowsTCPFailureReplaySuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	runID string
}

// TestWindowsTCPFailureReplay provisions a Windows EC2 host with NPM enabled,
// drops the Go HTTP replay binary onto it, and lets it run for trafficDuration.
//
// Run with:
//
//	dda inv new-e2e-tests.run --targets=./tests/windows/npm/... --run=TestWindowsTCPFailureReplay --keep-stack
//
// With --keep-stack the EC2 host is reused so iteration is cheap.
func TestWindowsTCPFailureReplay(t *testing.T) {
	t.Parallel()

	suite := &windowsTCPFailureReplaySuite{
		runID: fmt.Sprintf("tcp-failure-replay-%d", time.Now().Unix()),
	}

	uniqueTag := "e2e_test_run:" + suite.runID
	provisioner := awsHostWindows.ProvisionerNoFakeIntake(
		awsHostWindows.WithRunOptions(
			scenwin.WithAgentOptions(
				agentparams.WithSystemProbeConfig(systemProbeConfigNPM),
				agentparams.WithTags([]string{uniqueTag}),
			),
		),
	)

	e2e.Run(t, suite, e2e.WithProvisioner(provisioner))
}

// TestGenerateTCPFailures cross-compiles the Go HTTP replay binary, uploads
// it, runs it for trafficDuration, then reads back the log so we can see
// exactly how many connections and how much data the run produced.
func (s *windowsTCPFailureReplaySuite) TestGenerateTCPFailures() {
	t := s.T()
	host := s.Env().RemoteHost

	hostname, err := windowsCommon.GetHostname(host)
	s.Require().NoError(err, "should fetch hostname")
	t.Logf("================ TCP FAILURE REPLAY (TLS/HTTP) ================")
	t.Logf("  hostname        : %s", hostname)
	t.Logf("  unique tag      : e2e_test_run:%s", s.runID)
	t.Logf("  start (UTC)     : %s", time.Now().UTC().Format(time.RFC3339))
	t.Logf("  traffic duration: %s", trafficDuration)
	t.Logf("  work dir on host: %s", workDir)
	t.Logf("  server port     : %d", serverPort)
	t.Logf("  client target   : %s:%d (real NIC, not loopback)", clientTargetIP, serverPort)
	t.Logf("================================================================")

	// 1. NPM services up.
	t.Log("Waiting for ddnpm, datadog-system-probe, datadogagent to reach Running...")
	for _, svc := range []string{"ddnpm", "datadog-system-probe", "datadogagent"} {
		s.Require().EventuallyWithT(func(c *assert.CollectT) {
			status, err := windowsCommon.GetServiceStatus(host, svc)
			assert.NoError(c, err, "GetServiceStatus(%s)", svc)
			assert.Equal(c, "Running", status, "service %s not running", svc)
		}, 2*time.Minute, 2*time.Second, "service %s should be running", svc)
	}
	t.Log("All NPM services are Running.")

	// 2. Cleanup any leftover replay processes from a previous iteration.
	// IMPORTANT: also kill leftover PowerShell rst-server.ps1 (from v3/v4)
	// in case they're still holding port 19999.
	t.Log("Cleaning up any leftover processes (tcp-rst-replay + rst-server.ps1)...")
	_, _ = host.Execute(`powershell -NoProfile -Command "Get-Process tcp-rst-replay -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue"`)
	_, _ = host.Execute(`powershell -NoProfile -Command "Get-Process powershell -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -match 'rst-server.ps1' } | Stop-Process -Force -ErrorAction SilentlyContinue"`)
	_, err = host.Execute(fmt.Sprintf(
		`powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '%s' | Out-Null"`,
		workDir,
	))
	s.Require().NoError(err, "should create work dir")

	// 3. Cross-compile + upload the Go binary.
	t.Log("Cross-compiling Go HTTP replay binary (windows/amd64)...")
	binaryPath := buildReplayBinary(s)
	defer os.Remove(binaryPath)

	remoteBinary := workDir + `\tcp-rst-replay.exe`
	t.Logf("Uploading replay binary to %s...", remoteBinary)
	host.CopyFile(binaryPath, remoteBinary)

	// 4. Launch server AND run client in a SINGLE host.Execute call via a
	// PowerShell script file on the host.
	//
	// Why one session: OpenSSH on Windows puts every process spawned
	// during an SSH session into a Win32 Job Object and calls
	// TerminateJobObject when the session disconnects. Two separate
	// host.Execute calls (one for server, one for client) means the
	// server is killed between them — confirmed empirically: pid alive
	// at end of session 1, gone in session 2.
	//
	// Why a file (not -Command "..."): the framework wraps every
	// host.Execute call in `$ErrorActionPreference='Stop'; $LASTEXITCODE=0;
	// <cmd>; if (-not $?) { exit $LASTEXITCODE }` which is interpreted by
	// an outer PowerShell. That outer shell interpolates $variables inside
	// any double-quoted -Command argument BEFORE the inner powershell
	// reads them, so $server.Id and $clientExit get replaced with empty
	// strings and the script becomes unparseable. Invoking via -File
	// skips that whole layer.
	serverLog := workDir + `\tcp-rst-replay.server.log`
	durationFlag := fmt.Sprintf("%ds", int(trafficDuration.Seconds()))
	const clientWorkers = 256
	t.Logf("Launching server + running client in ONE SSH session via -File (tls=true, hold=0s, %d workers, %s)...", clientWorkers, durationFlag)

	scriptLocal, scriptRemote := buildRunReplayScript(s, runReplayScriptParams{
		ExePath:           remoteBinary,
		ServerPort:        serverPort,
		ClientTargetIP:    clientTargetIP,
		ServerLog:         serverLog,
		Hold:              "0s",
		Workers:           clientWorkers,
		Duration:          durationFlag,
		ServerStartupWait: "3",
	})
	defer os.Remove(scriptLocal)
	host.CopyFile(scriptLocal, scriptRemote)

	clientStdout, clientErr := host.Execute(fmt.Sprintf(
		`powershell -NoProfile -ExecutionPolicy Bypass -File '%s'`, scriptRemote,
	))
	if clientErr != nil {
		t.Logf("Combined server+client run FAILED: %v", clientErr)
	}
	t.Logf("--- combined stdout (full) ---\n%s\n--- end ---", trimMaxLen(clientStdout, 10000))

	// 6. Agent flush slack — wait for any final NPM payload to ship.
	t.Log("Sleeping 60s for final agent flush...")
	time.Sleep(60 * time.Second)

	// 7. Stop the Go server.
	_, _ = host.Execute(`powershell -NoProfile -Command "Get-Process tcp-rst-replay -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue"`)

	// 7. Diagnostic: list everything in workDir + check process state, then
	// surface both log files.
	dirOut, _ := host.Execute(fmt.Sprintf(
		`powershell -NoProfile -Command "Get-ChildItem -Path '%s' | Select-Object Name,Length,LastWriteTime | Format-Table -AutoSize | Out-String"`,
		workDir,
	))
	t.Logf("--- workDir listing ---\n%s\n--- end ---", dirOut)

	psOut, _ := host.Execute(`powershell -NoProfile -Command "Get-Process tcp-rst-replay -ErrorAction SilentlyContinue | Select-Object Id,ProcessName,StartTime | Format-Table -AutoSize | Out-String; if (-not (Get-Process tcp-rst-replay -ErrorAction SilentlyContinue)) { 'no tcp-rst-replay processes running' }"`)
	t.Logf("--- tcp-rst-replay processes ---\n%s\n--- end ---", psOut)

	// Try to read the server log; report explicitly if missing or empty.
	srvLogOut, _ := host.Execute(fmt.Sprintf(
		`powershell -NoProfile -Command "if (Test-Path '%s') { $sz = (Get-Item '%s').Length; if ($sz -eq 0) { 'FILE EXISTS BUT IS EMPTY (0 bytes)' } else { Get-Content -Path '%s' -Tail 50 } } else { 'FILE DOES NOT EXIST' }"`,
		serverLog, serverLog, serverLog,
	))
	t.Logf("--- %s ---\n%s\n--- end ---", serverLog, srvLogOut)

	endUTC := time.Now().UTC()
	t.Logf("================ DONE — VERIFY ON BACKEND ================")
	t.Logf("  hostname    : %s", hostname)
	t.Logf("  end (UTC)   : %s", endUTC.Format(time.RFC3339))
	t.Logf("")
	t.Logf("  --keep-stack tag-staleness caveat:")
	t.Logf("    The agent on this host has NOT been restarted since first")
	t.Logf("    provision, so its e2e_test_run host tag is stuck on the")
	t.Logf("    first iteration's value, NOT the one printed above.")
	t.Logf("    Filter NPM by:  host:%s + time:[start,end]", hostname)
	t.Logf("    Server port  :  %d", serverPort)
	t.Logf("  Chart the per-host TCP failure rate. Bug = rate > 100%%.")
	t.Logf("==========================================================")
}

// trimMaxLen truncates s for safe inclusion in test logs.
func trimMaxLen(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

type runReplayScriptParams struct {
	ExePath           string
	ServerPort        int
	ClientTargetIP    string
	ServerLog         string
	Hold              string
	Workers           int
	Duration          string
	ServerStartupWait string // seconds, as a string (e.g. "3")
}

// runReplayScriptTmpl is the PowerShell that launches server, sleeps, runs
// client foreground, stops server. We do NOT use template parameter syntax
// for the $vars — they're real PowerShell variables. Only the values we
// supply (paths, ports, IPs, durations) are templated via fmt.Sprintf.
const runReplayScriptTmpl = `$ErrorActionPreference = 'Continue'
$exe          = '%[1]s'
$serverPort   = %[2]d
$clientTarget = '%[3]s:%[2]d'
$serverLog    = '%[4]s'
$hold         = '%[5]s'
$workers      = %[6]d
$duration     = '%[7]s'
$wait         = %[8]s

Write-Host "[harness] starting server via Start-Process -PassThru"
$server = Start-Process -FilePath $exe -ArgumentList @(
    '-mode=server',
    "-bind=0.0.0.0:$serverPort",
    '-tls=true',
    "-hold=$hold",
    "-log=$serverLog"
) -WindowStyle Hidden -PassThru
if (-not $server) {
    Write-Host '[harness] ERROR: Start-Process returned no process'
    exit 2
}
Write-Host ('[harness] server pid=' + $server.Id)

Start-Sleep -Seconds $wait

if (-not (Get-Process -Id $server.Id -ErrorAction SilentlyContinue)) {
    Write-Host '[harness] ERROR: server died during startup wait'
    if (Test-Path $serverLog) { Get-Content $serverLog -Raw | Write-Host }
    exit 3
}

Write-Host "[harness] running client foreground (target=$clientTarget)"
& $exe '-mode=client' "-target=$clientTarget" '-tls=true' "-workers=$workers" "-duration=$duration" '-stdout' 2>&1
$clientExit = $LASTEXITCODE
Write-Host ('[harness] client exit=' + $clientExit)

if (Get-Process -Id $server.Id -ErrorAction SilentlyContinue) {
    Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
}
$null = Wait-Process -Id $server.Id -Timeout 5 -ErrorAction SilentlyContinue

Write-Host '[harness] server log tail (last 30 lines):'
if (Test-Path $serverLog) { Get-Content $serverLog -Tail 30 } else { Write-Host '(server log not found)' }

exit $clientExit
`

// buildRunReplayScript writes the PowerShell run script to a local temp file
// and returns (localPath, remotePath). Caller is responsible for copying
// localPath to remotePath on the host.
func buildRunReplayScript(s *windowsTCPFailureReplaySuite, p runReplayScriptParams) (string, string) {
	t := s.T()
	tmpDir, err := os.MkdirTemp("", "tcp-rst-replay-script-*")
	s.Require().NoError(err, "should create temp dir")
	localPath := filepath.Join(tmpDir, "run-replay.ps1")
	content := fmt.Sprintf(runReplayScriptTmpl,
		p.ExePath, p.ServerPort, p.ClientTargetIP, p.ServerLog, p.Hold, p.Workers, p.Duration, p.ServerStartupWait)
	s.Require().NoError(os.WriteFile(localPath, []byte(content), 0644))
	remotePath := workDir + `\run-replay.ps1`
	t.Logf("  wrote run script: %s (%d bytes) → %s", localPath, len(content), remotePath)
	return localPath, remotePath
}

// buildReplayBinary writes goReplayClientSource to a temp file, cross-
// compiles it for windows/amd64, and returns the path to the resulting .exe.
func buildReplayBinary(s *windowsTCPFailureReplaySuite) string {
	t := s.T()
	tmpDir, err := os.MkdirTemp("", "tcp-rst-replay-*")
	s.Require().NoError(err, "should create temp dir")

	srcPath := filepath.Join(tmpDir, "main.go")
	s.Require().NoError(os.WriteFile(srcPath, []byte(goReplayClientSource), 0644))

	exePath := filepath.Join(tmpDir, "tcp-rst-replay.exe")
	cmd := exec.Command("go", "build", "-trimpath", "-o", exePath, srcPath)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "go build failed: %s", string(out))

	info, err := os.Stat(exePath)
	s.Require().NoError(err, "binary should exist")
	t.Logf("  built %s (%d bytes)", exePath, info.Size())
	return exePath
}

// Retained from earlier revisions for completeness; not used by v5 directly,
// but kept available so PowerShell-based scaffolding can be reintroduced if
// we need to mix protocols in a future iteration.
var _ = startDetached

func startDetached(s *windowsTCPFailureReplaySuite, host *components.RemoteHost, name, script string) {
	t := s.T()
	scriptPath := fmt.Sprintf(`%s\%s.ps1`, workDir, name)
	logPath := fmt.Sprintf(`%s\%s.log`, workDir, name)

	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	_, err := host.Execute(fmt.Sprintf(
		`powershell -NoProfile -Command "[IO.File]::WriteAllBytes('%s', [Convert]::FromBase64String('%s'))"`,
		scriptPath, encoded,
	))
	s.Require().NoError(err, "should write %s script", name)

	launchCmd := fmt.Sprintf(
		`powershell -NoProfile -Command "Start-Process -FilePath powershell.exe -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File','%s') -WindowStyle Hidden -RedirectStandardOutput '%s' -RedirectStandardError '%s.err'"`,
		scriptPath, logPath, logPath,
	)
	_, err = host.Execute(launchCmd)
	s.Require().NoError(err, "should start %s detached", name)
	t.Logf("  started %s (script=%s log=%s)", name, scriptPath, logPath)
}
