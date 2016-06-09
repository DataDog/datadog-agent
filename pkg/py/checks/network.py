"""
Collects network metrics.
"""
# stdlib
import re

# project
from checks import AgentCheck
from utils.platform import Platform
from utils.subprocess_output import get_subprocess_output

BSD_TCP_METRICS = [
    (re.compile("^\s*(\d+) data packets \(\d+ bytes\) retransmitted\s*$"), 'system.net.tcp.retrans_packs'),
    (re.compile("^\s*(\d+) packets sent\s*$"), 'system.net.tcp.sent_packs'),
    (re.compile("^\s*(\d+) packets received\s*$"), 'system.net.tcp.rcv_packs')
]

SOLARIS_TCP_METRICS = [
    (re.compile("\s*tcpRetransSegs\s*=\s*(\d+)\s*"), 'system.net.tcp.retrans_segs'),
    (re.compile("\s*tcpOutDataSegs\s*=\s*(\d+)\s*"), 'system.net.tcp.in_segs'),
    (re.compile("\s*tcpInSegs\s*=\s*(\d+)\s*"), 'system.net.tcp.out_segs')
]


class Network(AgentCheck):

    SOURCE_TYPE_NAME = 'system'

    TCP_STATES = {
        "ss": {
            "ESTAB": "established",
            "SYN-SENT": "opening",
            "SYN-RECV": "opening",
            "FIN-WAIT-1": "closing",
            "FIN-WAIT-2": "closing",
            "TIME-WAIT": "time_wait",
            "UNCONN": "closing",
            "CLOSE-WAIT": "closing",
            "LAST-ACK": "closing",
            "LISTEN": "listening",
            "CLOSING": "closing",
        },
        "netstat": {
            "ESTABLISHED": "established",
            "SYN_SENT": "opening",
            "SYN_RECV": "opening",
            "FIN_WAIT1": "closing",
            "FIN_WAIT2": "closing",
            "TIME_WAIT": "time_wait",
            "CLOSE": "closing",
            "CLOSE_WAIT": "closing",
            "LAST_ACK": "closing",
            "LISTEN": "listening",
            "CLOSING": "closing",
        }
    }

    CX_STATE_GAUGE = {
        ('udp4', 'connections') : 'system.net.udp4.connections',
        ('udp6', 'connections') : 'system.net.udp6.connections',
        ('tcp4', 'established') : 'system.net.tcp4.established',
        ('tcp4', 'opening') : 'system.net.tcp4.opening',
        ('tcp4', 'closing') : 'system.net.tcp4.closing',
        ('tcp4', 'listening') : 'system.net.tcp4.listening',
        ('tcp4', 'time_wait') : 'system.net.tcp4.time_wait',
        ('tcp6', 'established') : 'system.net.tcp6.established',
        ('tcp6', 'opening') : 'system.net.tcp6.opening',
        ('tcp6', 'closing') : 'system.net.tcp6.closing',
        ('tcp6', 'listening') : 'system.net.tcp6.listening',
        ('tcp6', 'time_wait') : 'system.net.tcp6.time_wait',
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances=instances)
        if instances is not None and len(instances) > 1:
            raise Exception("Network check only supports one configured instance.")

    def check(self, instance):
        if instance is None:
            instance = {}

        self._excluded_ifaces = instance.get('excluded_interfaces', [])
        self._collect_cx_state = instance.get('collect_connection_state', False)

        self._exclude_iface_re = None
        exclude_re = instance.get('excluded_interface_re', None)
        if exclude_re:
            self.log.debug("Excluding network devices matching: %s" % exclude_re)
            self._exclude_iface_re = re.compile(exclude_re)

        if Platform.is_linux():
            self._check_linux(instance)
        elif Platform.is_bsd():
            self._check_bsd(instance)
        elif Platform.is_solaris():
            self._check_solaris(instance)

    def _submit_devicemetrics(self, iface, vals_by_metric):
        if iface in self._excluded_ifaces or (self._exclude_iface_re and self._exclude_iface_re.match(iface)):
            # Skip this network interface.
            return False

        expected_metrics = [
            'bytes_rcvd',
            'bytes_sent',
            'packets_in.count',
            'packets_in.error',
            'packets_out.count',
            'packets_out.error',
        ]
        for m in expected_metrics:
            assert m in vals_by_metric
        assert len(vals_by_metric) == len(expected_metrics)

        count = 0
        for metric, val in vals_by_metric.iteritems():
            self.rate('system.net.%s' % metric, val, device_name=iface)
            count += 1
        self.log.debug("tracked %s network metrics for interface %s" % (count, iface))

    def _parse_value(self, v):
        if v == "-":
            return 0
        else:
            try:
                return long(v)
            except ValueError:
                return 0

    def _submit_regexed_values(self, output, regex_list):
        lines = output.splitlines()
        for line in lines:
            for regex, metric in regex_list:
                value = re.match(regex, line)
                if value:
                    self.rate(metric, self._parse_value(value.group(1)))

    def _check_linux(self, instance):
        if self._collect_cx_state:
            try:
                self.log.debug("Using `ss` to collect connection state")
                # Try using `ss` for increased performance over `netstat`
                for ip_version in ['4', '6']:
                    # Call `ss` for each IP version because there's no built-in way of distinguishing
                    # between the IP versions in the output
                    output, _, _ = get_subprocess_output(["ss", "-n", "-u", "-t", "-a", "-{0}".format(ip_version)], self.log)
                    lines = output.splitlines()
                    # Netid  State      Recv-Q Send-Q     Local Address:Port       Peer Address:Port
                    # udp    UNCONN     0      0              127.0.0.1:8125                  *:*
                    # udp    ESTAB      0      0              127.0.0.1:37036         127.0.0.1:8125
                    # udp    UNCONN     0      0        fe80::a00:27ff:fe1c:3c4:123          :::*
                    # tcp    TIME-WAIT  0      0          90.56.111.177:56867        46.105.75.4:143
                    # tcp    LISTEN     0      0       ::ffff:127.0.0.1:33217  ::ffff:127.0.0.1:7199
                    # tcp    ESTAB      0      0       ::ffff:127.0.0.1:58975  ::ffff:127.0.0.1:2181

                    metrics = self._parse_linux_cx_state(lines[1:], self.TCP_STATES['ss'], 1, ip_version=ip_version)
                    # Only send the metrics which match the loop iteration's ip version
                    for stat, metric in self.CX_STATE_GAUGE.iteritems():
                        if stat[0].endswith(ip_version):
                            self.gauge(metric, metrics.get(metric))

            except OSError:
                self.log.info("`ss` not found: using `netstat` as a fallback")
                output, _, _ = get_subprocess_output(["netstat", "-n", "-u", "-t", "-a"], self.log)
                lines = output.splitlines()
                # Active Internet connections (w/o servers)
                # Proto Recv-Q Send-Q Local Address           Foreign Address         State
                # tcp        0      0 46.105.75.4:80          79.220.227.193:2032     SYN_RECV
                # tcp        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
                # tcp        0      0 46.105.75.4:50468       107.20.207.175:443      TIME_WAIT
                # tcp6       0      0 46.105.75.4:80          93.15.237.188:58038     FIN_WAIT2
                # tcp6       0      0 46.105.75.4:80          79.220.227.193:2029     ESTABLISHED
                # udp        0      0 0.0.0.0:123             0.0.0.0:*
                # udp6       0      0 :::41458                :::*

                metrics = self._parse_linux_cx_state(lines[2:], self.TCP_STATES['netstat'], 5)
                for metric, value in metrics.iteritems():
                    self.gauge(metric, value)

        proc = open('/proc/net/dev', 'r')
        try:
            lines = proc.readlines()
        finally:
            proc.close()
        # Inter-|   Receive                                                 |  Transmit
        #  face |bytes     packets errs drop fifo frame compressed multicast|bytes       packets errs drop fifo colls carrier compressed
        #     lo:45890956   112797   0    0    0     0          0         0    45890956   112797    0    0    0     0       0          0
        #   eth0:631947052 1042233   0   19    0   184          0      1206  1208625538  1320529    0    0    0     0       0          0
        #   eth1:       0        0   0    0    0     0          0         0           0        0    0    0    0     0       0          0
        for l in lines[2:]:
            cols = l.split(':', 1)
            x = cols[1].split()
            # Filter inactive interfaces
            if self._parse_value(x[0]) or self._parse_value(x[8]):
                iface = cols[0].strip()
                metrics = {
                    'bytes_rcvd': self._parse_value(x[0]),
                    'bytes_sent': self._parse_value(x[8]),
                    'packets_in.count': self._parse_value(x[1]),
                    'packets_in.error': self._parse_value(x[2]) + self._parse_value(x[3]),
                    'packets_out.count': self._parse_value(x[9]),
                    'packets_out.error':self._parse_value(x[10]) + self._parse_value(x[11]),
                }
                self._submit_devicemetrics(iface, metrics)

        try:
            proc = open('/proc/net/snmp', 'r')

            # IP:      Forwarding   DefaultTTL InReceives     InHdrErrors  ...
            # IP:      2            64         377145470      0            ...
            # Icmp:    InMsgs       InErrors   InDestUnreachs InTimeExcds  ...
            # Icmp:    1644495      1238       1643257        0            ...
            # IcmpMsg: InType3      OutType3
            # IcmpMsg: 1643257      1643257
            # Tcp:     RtoAlgorithm RtoMin     RtoMax         MaxConn      ...
            # Tcp:     1            200        120000         -1           ...
            # Udp:     InDatagrams  NoPorts    InErrors       OutDatagrams ...
            # Udp:     24249494     1643257    0              25892947     ...
            # UdpLite: InDatagrams  Noports    InErrors       OutDatagrams ...
            # UdpLite: 0            0          0              0            ...
            try:
                lines = proc.readlines()
            finally:
                proc.close()

            tcp_lines = [line for line in lines if line.startswith('Tcp:')]
            udp_lines = [line for line in lines if line.startswith('Udp:')]

            tcp_column_names = tcp_lines[0].strip().split()
            tcp_values = tcp_lines[1].strip().split()
            tcp_metrics = dict(zip(tcp_column_names, tcp_values))

            udp_column_names = udp_lines[0].strip().split()
            udp_values = udp_lines[1].strip().split()
            udp_metrics = dict(zip(udp_column_names, udp_values))

            # line start indicating what kind of metrics we're looking at
            assert(tcp_metrics['Tcp:'] == 'Tcp:')

            tcp_metrics_name = {
                'RetransSegs': 'system.net.tcp.retrans_segs',
                'InSegs'     : 'system.net.tcp.in_segs',
                'OutSegs'    : 'system.net.tcp.out_segs'
            }

            for key, metric in tcp_metrics_name.iteritems():
                self.rate(metric, self._parse_value(tcp_metrics[key]))

            assert(udp_metrics['Udp:'] == 'Udp:')

            udp_metrics_name = {
                'InDatagrams': 'system.net.udp.in_datagrams',
                'NoPorts': 'system.net.udp.no_ports',
                'InErrors': 'system.net.udp.in_errors',
                'OutDatagrams': 'system.net.udp.out_datagrams',
                'RcvbufErrors': 'system.net.udp.rcv_buf_errors',
                'SndbufErrors': 'system.net.udp.snd_buf_errors'
            }
            for key, metric in udp_metrics_name.iteritems():
                if key in udp_metrics:
                    self.rate(metric, self._parse_value(udp_metrics[key]))

        except IOError:
            # On Openshift, /proc/net/snmp is only readable by root
            self.log.debug("Unable to read /proc/net/snmp.")

    # Parse the output of the command that retrieves the connection state (either `ss` or `netstat`)
    # Returns a dict metric_name -> value
    def _parse_linux_cx_state(self, lines, tcp_states, state_col, ip_version=None):
        metrics = dict.fromkeys(self.CX_STATE_GAUGE.values(), 0)
        for l in lines:
            cols = l.split()
            if cols[0].startswith('tcp'):
                protocol = "tcp{0}".format(ip_version) if ip_version else ("tcp4", "tcp6")[cols[0] == "tcp6"]
                if cols[state_col] in tcp_states:
                    metric = self.CX_STATE_GAUGE[protocol, tcp_states[cols[state_col]]]
                    metrics[metric] += 1
            elif cols[0].startswith('udp'):
                protocol = "udp{0}".format(ip_version) if ip_version else ("udp4", "udp6")[cols[0] == "udp6"]
                metric = self.CX_STATE_GAUGE[protocol, 'connections']
                metrics[metric] += 1

        return metrics

    def _check_bsd(self, instance):
        netstat_flags = ['-i', '-b']

        # FreeBSD's netstat truncates device names unless you pass '-W'
        if Platform.is_freebsd():
            netstat_flags.append('-W')

        output, _, _ = get_subprocess_output(["netstat"] + netstat_flags, self.log)
        lines = output.splitlines()
        # Name  Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
        # lo0   16384 <Link#1>                        318258     0  428252203   318258     0  428252203     0
        # lo0   16384 localhost   fe80:1::1           318258     -  428252203   318258     -  428252203     -
        # lo0   16384 127           localhost         318258     -  428252203   318258     -  428252203     -
        # lo0   16384 localhost   ::1                 318258     -  428252203   318258     -  428252203     -
        # gif0* 1280  <Link#2>                             0     0          0        0     0          0     0
        # stf0* 1280  <Link#3>                             0     0          0        0     0          0     0
        # en0   1500  <Link#4>    04:0c:ce:db:4e:fa 20801309     0 13835457425 15149389     0 11508790198     0
        # en0   1500  seneca.loca fe80:4::60c:ceff: 20801309     - 13835457425 15149389     - 11508790198     -
        # en0   1500  2001:470:1f 2001:470:1f07:11d 20801309     - 13835457425 15149389     - 11508790198     -
        # en0   1500  2001:470:1f 2001:470:1f07:11d 20801309     - 13835457425 15149389     - 11508790198     -
        # en0   1500  192.168.1     192.168.1.63    20801309     - 13835457425 15149389     - 11508790198     -
        # en0   1500  2001:470:1f 2001:470:1f07:11d 20801309     - 13835457425 15149389     - 11508790198     -
        # p2p0  2304  <Link#5>    06:0c:ce:db:4e:fa        0     0          0        0     0          0     0
        # ham0  1404  <Link#6>    7a:79:05:4d:bf:f5    30100     0    6815204    18742     0    8494811     0
        # ham0  1404  5             5.77.191.245       30100     -    6815204    18742     -    8494811     -
        # ham0  1404  seneca.loca fe80:6::7879:5ff:    30100     -    6815204    18742     -    8494811     -
        # ham0  1404  2620:9b::54 2620:9b::54d:bff5    30100     -    6815204    18742     -    8494811     -

        headers = lines[0].split()

        # Given the irregular structure of the table above, better to parse from the end of each line
        # Verify headers first
        #          -7       -6       -5        -4       -3       -2        -1
        for h in ("Ipkts", "Ierrs", "Ibytes", "Opkts", "Oerrs", "Obytes", "Coll"):
            if h not in headers:
                self.logger.error("%s not found in %s; cannot parse" % (h, headers))
                return False

        current = None
        for l in lines[1:]:
            # Another header row, abort now, this is IPv6 land
            if "Name" in l:
                break

            x = l.split()
            if len(x) == 0:
                break

            iface = x[0]
            if iface.endswith("*"):
                iface = iface[:-1]
            if iface == current:
                # skip multiple lines of same interface
                continue
            else:
                current = iface

            # Filter inactive interfaces
            if self._parse_value(x[-5]) or self._parse_value(x[-2]):
                iface = current
                metrics = {
                    'bytes_rcvd': self._parse_value(x[-5]),
                    'bytes_sent': self._parse_value(x[-2]),
                    'packets_in.count': self._parse_value(x[-7]),
                    'packets_in.error': self._parse_value(x[-6]),
                    'packets_out.count': self._parse_value(x[-4]),
                    'packets_out.error':self._parse_value(x[-3]),
                }
                self._submit_devicemetrics(iface, metrics)


        netstat, _, _ = get_subprocess_output(["netstat", "-s", "-p" "tcp"], self.log)
        #3651535 packets sent
        #        972097 data packets (615753248 bytes)
        #        5009 data packets (2832232 bytes) retransmitted
        #        0 resends initiated by MTU discovery
        #        2086952 ack-only packets (471 delayed)
        #        0 URG only packets
        #        0 window probe packets
        #        310851 window update packets
        #        336829 control packets
        #        0 data packets sent after flow control
        #        3058232 checksummed in software
        #        3058232 segments (571218834 bytes) over IPv4
        #        0 segments (0 bytes) over IPv6
        #4807551 packets received
        #        1143534 acks (for 616095538 bytes)
        #        165400 duplicate acks
        #        ...

        self._submit_regexed_values(netstat, BSD_TCP_METRICS)


    def _check_solaris(self, instance):
        # Can't get bytes sent and received via netstat
        # Default to kstat -p link:0:
        netstat, _, _ = get_subprocess_output(["kstat", "-p", "link:0:"], self.log)
        metrics_by_interface = self._parse_solaris_netstat(netstat)
        for interface, metrics in metrics_by_interface.iteritems():
            self._submit_devicemetrics(interface, metrics)

        netstat, _, _ = get_subprocess_output(["netstat", "-s", "-P" "tcp"], self.log)
        # TCP: tcpRtoAlgorithm=     4 tcpRtoMin           =   200
        # tcpRtoMax           = 60000 tcpMaxConn          =    -1
        # tcpActiveOpens      =    57 tcpPassiveOpens     =    50
        # tcpAttemptFails     =     1 tcpEstabResets      =     0
        # tcpCurrEstab        =     0 tcpOutSegs          =   254
        # tcpOutDataSegs      =   995 tcpOutDataBytes     =1216733
        # tcpRetransSegs      =     0 tcpRetransBytes     =     0
        # tcpOutAck           =   185 tcpOutAckDelayed    =     4
        # ...
        self._submit_regexed_values(netstat, SOLARIS_TCP_METRICS)


    def _parse_solaris_netstat(self, netstat_output):
        """
        Return a mapping of network metrics by interface. For example:
            { interface:
                {'bytes_sent': 0,
                  'bytes_rcvd': 0,
                  'bytes_rcvd': 0,
                  ...
                }
            }
        """
        # Here's an example of the netstat output:
        #
        # link:0:net0:brdcstrcv   527336
        # link:0:net0:brdcstxmt   1595
        # link:0:net0:class       net
        # link:0:net0:collisions  0
        # link:0:net0:crtime      16359935.2637943
        # link:0:net0:ierrors     0
        # link:0:net0:ifspeed     10000000000
        # link:0:net0:ipackets    682834
        # link:0:net0:ipackets64  682834
        # link:0:net0:link_duplex 0
        # link:0:net0:link_state  1
        # link:0:net0:multircv    0
        # link:0:net0:multixmt    1595
        # link:0:net0:norcvbuf    0
        # link:0:net0:noxmtbuf    0
        # link:0:net0:obytes      12820668
        # link:0:net0:obytes64    12820668
        # link:0:net0:oerrors     0
        # link:0:net0:opackets    105445
        # link:0:net0:opackets64  105445
        # link:0:net0:rbytes      113983614
        # link:0:net0:rbytes64    113983614
        # link:0:net0:snaptime    16834735.1607669
        # link:0:net0:unknowns    0
        # link:0:net0:zonename    53aa9b7e-48ba-4152-a52b-a6368c3d9e7c
        # link:0:net1:brdcstrcv   4947620
        # link:0:net1:brdcstxmt   1594
        # link:0:net1:class       net
        # link:0:net1:collisions  0
        # link:0:net1:crtime      16359935.2839167
        # link:0:net1:ierrors     0
        # link:0:net1:ifspeed     10000000000
        # link:0:net1:ipackets    4947620
        # link:0:net1:ipackets64  4947620
        # link:0:net1:link_duplex 0
        # link:0:net1:link_state  1
        # link:0:net1:multircv    0
        # link:0:net1:multixmt    1594
        # link:0:net1:norcvbuf    0
        # link:0:net1:noxmtbuf    0
        # link:0:net1:obytes      73324
        # link:0:net1:obytes64    73324
        # link:0:net1:oerrors     0
        # link:0:net1:opackets    1594
        # link:0:net1:opackets64  1594
        # link:0:net1:rbytes      304384894
        # link:0:net1:rbytes64    304384894
        # link:0:net1:snaptime    16834735.1613302
        # link:0:net1:unknowns    0
        # link:0:net1:zonename    53aa9b7e-48ba-4152-a52b-a6368c3d9e7c

        # A mapping of solaris names -> datadog names
        metric_by_solaris_name = {
            'rbytes64':'bytes_rcvd',
            'obytes64':'bytes_sent',
            'ipackets64':'packets_in.count',
            'ierrors':'packets_in.error',
            'opackets64':'packets_out.count',
            'oerrors':'packets_out.error',
        }

        lines = [l for l in netstat_output.splitlines() if len(l) > 0]

        metrics_by_interface = {}

        for l in lines:
            # Parse the metric & interface.
            cols = l.split()
            link, n, iface, name = cols[0].split(":")
            assert link == "link"

            # Get the datadog metric name.
            ddname = metric_by_solaris_name.get(name, None)
            if ddname is None:
                continue

            # Add it to this interface's list of metrics.
            metrics = metrics_by_interface.get(iface, {})
            metrics[ddname] = self._parse_value(cols[1])
            metrics_by_interface[iface] = metrics

        return metrics_by_interface
