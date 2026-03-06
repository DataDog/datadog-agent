// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// pingStats tracks RTT statistics for the ping summary.
type pingStats struct {
	sent     int
	received int
	rtts     []float64 // round-trip times in milliseconds
}

func (s *pingStats) loss() float64 {
	if s.sent == 0 {
		return 0
	}
	return float64(s.sent-s.received) / float64(s.sent) * 100
}

func (s *pingStats) min() float64 {
	m := math.MaxFloat64
	for _, v := range s.rtts {
		if v < m {
			m = v
		}
	}
	return m
}

func (s *pingStats) max() float64 {
	var m float64
	for _, v := range s.rtts {
		if v > m {
			m = v
		}
	}
	return m
}

func (s *pingStats) avg() float64 {
	if len(s.rtts) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s.rtts {
		sum += v
	}
	return sum / float64(len(s.rtts))
}

func (s *pingStats) mdev() float64 {
	if len(s.rtts) == 0 {
		return 0
	}
	avg := s.avg()
	var sumDev float64
	for _, v := range s.rtts {
		sumDev += math.Abs(v - avg)
	}
	return sumDev / float64(len(s.rtts))
}

const pingMaxCount = 100

// builtinPing implements a minimal ping command.
// Supported flags: -c count, -W timeout (seconds).
// Safety: -f (flood) is rejected; -c is capped at 100.
func (r *Runner) builtinPing(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	count := 4    // default: send 4 packets
	timeout := 10 // -W: per-packet timeout in seconds

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-f":
			r.errf("ping: -f (flood) is not available in safe shell\n")
			exit.code = 2
			return exit
		case "-c":
			v := fp.value()
			if v == "" {
				r.errf("ping: option requires an argument -- 'c'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				r.errf("ping: invalid count: %q\n", v)
				exit.code = 2
				return exit
			}
			count = n
			if count > pingMaxCount {
				count = pingMaxCount
				r.errf("ping: count capped at %d\n", pingMaxCount)
			}
		case "-W":
			v := fp.value()
			if v == "" {
				r.errf("ping: option requires an argument -- 'W'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				r.errf("ping: invalid timeout: %q\n", v)
				exit.code = 2
				return exit
			}
			timeout = n
		default:
			r.errf("ping: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	targets := fp.args()
	if len(targets) != 1 {
		r.errf("usage: ping [-c count] [-W timeout] destination\n")
		exit.code = 2
		return exit
	}
	host := targets[0]

	// Resolve hostname to IPv4 address.
	ipAddr, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		r.errf("ping: %s: %v\n", host, err)
		exit.code = 2
		return exit
	}

	// Open ICMP connection: try unprivileged UDP first, fall back to raw.
	conn, listenNet, err := pingListenPacket()
	if err != nil {
		r.errf("ping: %v\n", err)
		exit.code = 2
		return exit
	}
	defer conn.Close()

	// Enable TTL in control messages so we can report it.
	if p := conn.IPv4PacketConn(); p != nil {
		_ = p.SetControlMessage(ipv4.FlagTTL, true)
	}

	r.outf("PING %s (%s) 56(84) bytes of data.\n", host, ipAddr.IP)

	id := os.Getpid() & 0xffff
	stats := &pingStats{}
	timeoutDur := time.Duration(timeout) * time.Second
	startTime := time.Now()

	for seq := 1; seq <= count; seq++ {
		select {
		case <-ctx.Done():
			goto summary
		default:
		}

		rtt, ttl, nbytes, err := pingOnce(conn, listenNet, *ipAddr, id, seq, timeoutDur)
		stats.sent++
		if err != nil {
			// Timeout or error: no output line for this seq.
		} else {
			stats.received++
			rttMs := float64(rtt.Microseconds()) / 1000.0
			stats.rtts = append(stats.rtts, rttMs)
			r.outf("%d bytes from %s: icmp_seq=%d ttl=%d time=%.1f ms\n",
				nbytes, ipAddr.IP, seq, ttl, rttMs)
		}

		// Wait 1 second between pings (except after the last one).
		if seq < count {
			select {
			case <-ctx.Done():
				goto summary
			case <-time.After(1 * time.Second):
			}
		}
	}

summary:
	totalTime := time.Since(startTime).Milliseconds()
	r.outf("\n--- %s ping statistics ---\n", host)
	r.outf("%d packets transmitted, %d received, %.0f%% packet loss, time %dms\n",
		stats.sent, stats.received, stats.loss(), totalTime)
	if stats.received > 0 {
		r.outf("rtt min/avg/max/mdev = %.3f/%.3f/%.3f/%.3f ms\n",
			stats.min(), stats.avg(), stats.max(), stats.mdev())
	}

	if stats.received == 0 {
		exit.code = 1
	} else if stats.received < stats.sent {
		exit.code = 1
	}
	return exit
}

// pingListenPacket tries unprivileged UDP-based ICMP first, then falls back
// to raw ICMP sockets (which require root or CAP_NET_RAW).
func pingListenPacket() (*icmp.PacketConn, string, error) {
	conn, err := icmp.ListenPacket("udp4", "")
	if err == nil {
		return conn, "udp4", nil
	}

	conn, err2 := icmp.ListenPacket("ip4:icmp", "")
	if err2 == nil {
		return conn, "ip4:icmp", nil
	}

	return nil, "", fmt.Errorf("unable to open ICMP socket: udp4: %v; raw: %v", err, err2)
}

// pingOnce sends a single ICMP Echo Request and waits for the matching reply.
func pingOnce(conn *icmp.PacketConn, listenNet string, dst net.IPAddr, id, seq int, timeout time.Duration) (rtt time.Duration, ttl int, nbytes int, err error) {
	// Build 56-byte payload with a timestamp for identification.
	payload := make([]byte, 56)
	binary.BigEndian.PutUint64(payload[:8], uint64(time.Now().UnixNano()))

	msg := &icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: payload,
		},
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshal: %v", err)
	}

	// Destination address depends on listen mode.
	var dstAddr net.Addr
	if listenNet == "udp4" {
		dstAddr = &net.UDPAddr{IP: dst.IP, Zone: dst.Zone}
	} else {
		dstAddr = &net.IPAddr{IP: dst.IP, Zone: dst.Zone}
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, 0, 0, fmt.Errorf("set deadline: %v", err)
	}

	start := time.Now()
	if _, err := conn.WriteTo(msgBytes, dstAddr); err != nil {
		return 0, 0, 0, fmt.Errorf("write: %v", err)
	}

	buf := make([]byte, 1500)
	for {
		var n int
		var cm *ipv4.ControlMessage

		if p := conn.IPv4PacketConn(); p != nil {
			n, cm, _, err = p.ReadFrom(buf)
		} else {
			n, _, err = conn.ReadFrom(buf)
		}
		if err != nil {
			return 0, 0, 0, err
		}
		rtt = time.Since(start)

		reply, parseErr := icmp.ParseMessage(1, buf[:n])
		if parseErr != nil {
			continue
		}

		if reply.Type != ipv4.ICMPTypeEchoReply {
			continue
		}

		echo, ok := reply.Body.(*icmp.Echo)
		if !ok {
			continue
		}

		// In UDP mode the kernel rewrites the ICMP ID to the ephemeral
		// port, so we can only reliably match on sequence number.
		if listenNet != "udp4" && echo.ID != id {
			continue
		}
		if echo.Seq != seq {
			continue
		}

		ttl = 64 // default fallback
		if cm != nil && cm.TTL > 0 {
			ttl = cm.TTL
		}

		// Report the ICMP payload size (header + data = n in UDP mode,
		// subtract 20-byte IP header in raw mode).
		nbytes = n
		if listenNet != "udp4" && n > 20 {
			nbytes = n - 20
		}

		return rtt, ttl, nbytes, nil
	}
}
