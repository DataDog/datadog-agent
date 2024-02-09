# routest

RouTest is a tool to help end-to-end testing of tracerouting tools, by crafting pre-defined responses to traceroute packets.

It works using NFQUEUE (hence iptables, hence it only works on Linux). The user needs to:
* set up an NFQUEUE rule, for example `iptables -A OUTPUT -p udp --dport 33434:33634 -d 8.8.8.8 -j NFQUEUE --queue-num 101`. This will instruct the kernel to pass any UDP packet with IP destination 8.8.8.8 and destination port in the range 33434-33634 to the user space
* create a JSON config file that contains a description of the packets and the expected responses. A sample `config.json` is provided.
* run `routest -c <config file> -q 101 -i lo` (where 101 is the number of the NFQUEUE specified in the iptables command above). This will receive the packets from the kernel, and craft a response according to the config file.

Once this is done, just run your traceroute, e.g.:

```
$ sudo dublin-traceroute -n1 8.8.8.8
WARNING: you are running this program as root. Consider setting the CAP_NET_RAW 
         capability and running as non-root user as a more secure alternative.
Starting dublin-traceroute
Traceroute from 0.0.0.0:12345 to 8.8.8.8:33434~33434 (probing 1 path, min TTL is 1, max TTL is 30, delay is 10 ms)
== Flow ID 33434 ==
1    1.2.3.4 (1.2.3.4), IP ID: 17022 RTT 4.729 ms  ICMP (type=11, code=0) 'TTL expired in transit', NAT ID: 0, flow hash: 52376
2    1.2.3.5 (1.2.3.5), IP ID: 30935 RTT 4.443 ms  ICMP (type=11, code=0) 'TTL expired in transit', NAT ID: 0, flow hash: 52376
3    1.2.3.6 (1.2.3.6), IP ID: 35202 RTT 4.117 ms  ICMP (type=11, code=0) 'TTL expired in transit', NAT ID: 0, flow hash: 52376
4    8.8.8.8 (dns.google), IP ID: 35204 RTT 3.545 ms  ICMP (type=3, code=3) 'Destination port unreachable', NAT ID: 0, flow hash: 52376
Saved JSON file to trace.json .
You can convert it to DOT by running python3 -m dublintraceroute plot trace.json
```


On the `routest` side, the output is:
```
$ sudo ./routest -c config.json -q 101 -i lo
INFO[0000] Loaded configuration: 
[
    {
        "dst": "8.8.8.8",
        "dst_port": 33434,
        "ttl": 1,
        "reply": {
            "src": "1.2.3.4",
            "icmp_type": 11,
            "icmp_code": 0
        }
    },
    {
        "dst": "8.8.8.8",
        "dst_port": 33434,
        "ttl": 2,
        "reply": {
            "src": "1.2.3.5",
            "icmp_type": 11,
            "icmp_code": 0
        }
    },
    {
        "dst": "8.8.8.8",
        "dst_port": 33434,
        "ttl": 3,
        "reply": {
            "src": "1.2.3.6",
            "icmp_type": 11,
            "icmp_code": 0
        }
    },
    {
        "dst": "8.8.8.8",
        "dst_port": 33434,
        "ttl": 4,
        "reply": {
            "src": "8.8.8.8",
            "icmp_type": 3,
            "icmp_code": 3
        }
    }
] 
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23210 Flags:2 FragOff:0 TTL:1 Proto:17 Checksum:36363 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc00008e9c0 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23210 next:<nil> prev:<nil>} 
DEBU[0001] Found match {Src:<nil> Dst:8.8.8.8 SrcPort:<nil> DstPort:33434 TTL:1 Reply:{Src:1.2.3.4 Dst:<nil> IcmpType:11 IcmpCode:0 Payload:[]}} 
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23209 Flags:2 FragOff:0 TTL:2 Proto:17 Checksum:36108 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc0000d01e0 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23209 next:<nil> prev:<nil>} 
DEBU[0001] Found match {Src:<nil> Dst:8.8.8.8 SrcPort:<nil> DstPort:33434 TTL:2 Reply:{Src:1.2.3.5 Dst:<nil> IcmpType:11 IcmpCode:0 Payload:[]}} 
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23208 Flags:2 FragOff:0 TTL:3 Proto:17 Checksum:35853 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc0000d0450 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23208 next:<nil> prev:<nil>} 
DEBU[0001] Found match {Src:<nil> Dst:8.8.8.8 SrcPort:<nil> DstPort:33434 TTL:3 Reply:{Src:1.2.3.6 Dst:<nil> IcmpType:11 IcmpCode:0 Payload:[]}} 
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23207 Flags:2 FragOff:0 TTL:4 Proto:17 Checksum:35598 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc00008eba0 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23207 next:<nil> prev:<nil>} 
DEBU[0001] Found match {Src:<nil> Dst:8.8.8.8 SrcPort:<nil> DstPort:33434 TTL:4 Reply:{Src:8.8.8.8 Dst:<nil> IcmpType:3 IcmpCode:3 Payload:[]}} 
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23206 Flags:2 FragOff:0 TTL:5 Proto:17 Checksum:35343 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc0000d0630 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23206 next:<nil> prev:<nil>} 
INFO[0001] Packet not matching                          
DEBU[0001] Matching packet: &{Version:4 HeaderLen:5 DiffServ:0 TotalLen:35 ID:23205 Flags:2 FragOff:0 TTL:6 Proto:17 Checksum:35088 Src:172.17.212.243 Dst:8.8.8.8 Options:[] next:0xc0000fa120 IPinICMP:false} >> &{Src:12345 Dst:33434 Len:15 Csum:23205 next:<nil> prev:<nil>} 
INFO[0001] Packet not matching                          
...
```
