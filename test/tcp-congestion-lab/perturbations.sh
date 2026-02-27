#!/bin/bash
# TCP Congestion Signal Lab — Perturbation Commands
#
# Run these from the HOST to exercise each congestion signal.
# The Datadog agent (system-probe) should be running and monitoring
# the tcp-lab-client and tcp-lab-server containers.
#
# Usage: ./perturbations.sh <scenario>

set -e

CLIENT="tcp-lab-client"
SERVER="tcp-lab-server"
SERVER_IP="172.28.0.10"

case "${1:-help}" in

  #=== BASELINE: clean traffic, no perturbations ===
  baseline)
    echo "=== Baseline: 30s clean iperf3 ==="
    echo "Expected signals: delivered>0, all loss/retransmit signals=0"
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30
    ;;

  #=== LOSS: random packet loss → exercises lost_out, retrans_out, bytes_retrans, rto_count ===
  loss)
    LOSS_PCT="${2:-5}"
    echo "=== Packet loss: ${LOSS_PCT}% on client egress ==="
    echo "Expected signals: max_lost_out>0, max_retrans_out>0, bytes_retrans>0, rto_count>0"
    docker exec $CLIENT tc qdisc add dev eth0 root netem loss "${LOSS_PCT}%"
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== HEAVY LOSS: high loss rate → exercises rto_count, max_ca_state=4 (Loss) ===
  heavy-loss)
    echo "=== Heavy packet loss: 20% on client egress ==="
    echo "Expected signals: rto_count>0, max_ca_state=4 (Loss), recovery_count>0"
    docker exec $CLIENT tc qdisc add dev eth0 root netem loss 20%
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== REORDER: packet reordering → exercises reord_seen, sacked_out ===
  reorder)
    echo "=== Packet reordering: 25% reorder, 50ms delay on client egress ==="
    echo "Expected signals: reord_seen>0, max_sacked_out>0"
    docker exec $CLIENT tc qdisc add dev eth0 root netem delay 50ms reorder 25% 50%
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== DELAY: high latency → exercises max_packets_out (larger window) ===
  delay)
    echo "=== High delay: 200ms RTT with 50ms jitter on client egress ==="
    echo "Expected signals: max_packets_out>0 (more segments in-flight due to BDP)"
    docker exec $CLIENT tc qdisc add dev eth0 root netem delay 100ms 25ms
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== DELAY+LOSS: realistic WAN conditions → exercises recovery_count, dsack_dups ===
  wan)
    echo "=== WAN simulation: 100ms delay + 2% loss on client egress ==="
    echo "Expected signals: recovery_count>0, dsack_dups possibly>0, max_sacked_out>0"
    docker exec $CLIENT tc qdisc add dev eth0 root netem delay 50ms 10ms loss 2%
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 60
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== ECN: ECN congestion marking → exercises delivered_ce ===
  ecn)
    echo "=== ECN marking: 10% ECN CE marking on client egress ==="
    echo "Expected signals: delivered_ce>0"
    # ECN is enabled via sysctls in docker-compose.yml (tcp_ecn=1 on both containers).
    # Verify ECN is active:
    echo "Verifying ECN enabled..."
    docker exec $CLIENT sysctl net.ipv4.tcp_ecn
    docker exec $SERVER sysctl net.ipv4.tcp_ecn
    # netem on CLIENT egress: marks outgoing data packets with CE instead of
    # dropping. Server sees CE, echoes ECE in ACKs, client increments delivered_ce.
    docker exec $CLIENT tc qdisc add dev eth0 root netem loss 10% ecn
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30 &
    IPERF_PID=$!
    # After a few seconds, verify ECN negotiation on the connection:
    sleep 3
    echo "Checking ECN negotiation (look for 'ecn' flag):"
    docker exec $CLIENT ss -ti dst $SERVER_IP:5201 | head -5
    wait $IPERF_PID || true
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== ZERO-WINDOW: slow reader → exercises probe0_count ===
  zero-window)
    echo "=== Zero-window: flood slow-reader server on :9000 ==="
    echo "Expected signals: probe0_count>0"
    echo "Server uses TCP_WINDOW_CLAMP=1 to force zero-window quickly."
    echo "Sending data to slow-reader server..."
    # The slow-reader server sets TCP_WINDOW_CLAMP=1 on accepted connections,
    # forcing the receiver to advertise window=0 almost immediately.
    # Send in background so we can monitor.
    docker exec $CLIENT sh -c "dd if=/dev/zero bs=4096 count=1000 2>/dev/null | nc -w 60 $SERVER_IP 9000 || true" &
    NC_PID=$!
    echo "Waiting 30s for zero-window probes to accumulate..."
    sleep 30
    kill $NC_PID 2>/dev/null || true
    wait $NC_PID 2>/dev/null || true
    echo "Done. Check probe0_count in Datadog."
    ;;

  #=== SACK: selective loss to trigger fast recovery → exercises recovery_count, sacked_out ===
  sack-recovery)
    echo "=== SACK recovery: 5% loss with delay to trigger fast recovery ==="
    echo "Expected signals: recovery_count>0, max_sacked_out>0, max_ca_state>=3"
    docker exec $CLIENT tc qdisc add dev eth0 root netem delay 50ms loss 5% 25%
    # Use parallel streams to ensure multiple segments in-flight
    docker exec $CLIENT iperf3 -c $SERVER_IP -p 5201 -t 30 -P 4
    docker exec $CLIENT tc qdisc del dev eth0 root
    ;;

  #=== ALL: run all scenarios sequentially ===
  all)
    for scenario in baseline loss heavy-loss reorder delay wan ecn zero-window sack-recovery; do
      echo ""
      echo "================================================"
      $0 $scenario
      echo "================================================"
      echo "Sleeping 10s between scenarios..."
      sleep 10
    done
    ;;

  #=== CLEANUP: remove any leftover netem rules ===
  cleanup)
    echo "=== Cleaning up tc rules ==="
    docker exec $CLIENT tc qdisc del dev eth0 root 2>/dev/null || true
    docker exec $SERVER tc qdisc del dev eth0 root 2>/dev/null || true
    echo "Done."
    ;;

  *)
    echo "TCP Congestion Signal Lab — Perturbation Scenarios"
    echo ""
    echo "Usage: $0 <scenario>"
    echo ""
    echo "Scenarios:"
    echo "  baseline       Clean traffic (no perturbations)"
    echo "  loss [%]       Random packet loss (default 5%)"
    echo "  heavy-loss     Heavy 20% loss (triggers RTO)"
    echo "  reorder        Packet reordering"
    echo "  delay          High latency (200ms RTT)"
    echo "  wan            WAN simulation (delay + loss)"
    echo "  ecn            ECN congestion marking"
    echo "  zero-window    Slow reader (zero-window probes)"
    echo "  sack-recovery  Selective loss (SACK fast recovery)"
    echo "  all            Run all scenarios sequentially"
    echo "  cleanup        Remove leftover tc rules"
    ;;
esac
