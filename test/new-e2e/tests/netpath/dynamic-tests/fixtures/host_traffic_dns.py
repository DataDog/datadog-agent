#!/usr/bin/env python3
import socket
import struct
import sys
import time

QUERY_TYPE_A = 1
QUERY_TYPE_AAAA = 28
QUERY_TYPE_ANY = 255
QUERY_CLASS_IN = 1


def parse_question(packet):
    if len(packet) < 12:
        raise ValueError("short DNS packet")

    labels = []
    offset = 12
    while True:
        if offset >= len(packet):
            raise ValueError("truncated query name")

        length = packet[offset]
        offset += 1
        if length == 0:
            break
        if length & 0xC0:
            raise ValueError("compressed query names are not supported")
        if offset + length > len(packet):
            raise ValueError("truncated query label")

        labels.append(packet[offset : offset + length].decode("ascii").lower())
        offset += length

    if offset + 4 > len(packet):
        raise ValueError("truncated DNS question")

    qtype, qclass = struct.unpack("!HH", packet[offset : offset + 4])
    return ".".join(labels), qtype, qclass, packet[12 : offset + 4]


def build_response(packet, question, ip_bytes=None, rcode=0):
    answer = ip_bytes is not None
    flags = 0x8180 | rcode
    response = packet[:2] + struct.pack("!HHHHH", flags, 1, 1 if answer else 0, 0, 0) + question
    if not answer:
        return response

    return response + b"\xc0\x0c" + struct.pack("!HHIH", QUERY_TYPE_A, QUERY_CLASS_IN, 30, len(ip_bytes)) + ip_bytes


def forward(packet, upstream):
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.settimeout(2)
        sock.sendto(packet, (upstream, 53))
        response, _ = sock.recvfrom(4096)
        return response


def main():
    if len(sys.argv) != 5:
        print(
            "usage: host_traffic_dns.py <bind-ip> <domain> <target-ip> <upstream-dns>",
            file=sys.stderr,
        )
        return 2

    bind_ip, domain, target_ip, upstream = sys.argv[1:]
    domain = domain.rstrip(".").lower()
    target_ip_bytes = socket.inet_aton(target_ip)

    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind((bind_ip, 53))
        print(
            f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} "
            f"serving {domain}={target_ip} on {bind_ip}:53, forwarding to {upstream}:53",
            flush=True,
        )

        while True:
            packet, addr = sock.recvfrom(4096)
            try:
                name, qtype, qclass, question = parse_question(packet)
                if name == domain and qclass == QUERY_CLASS_IN:
                    if qtype in (QUERY_TYPE_A, QUERY_TYPE_ANY):
                        response = build_response(packet, question, target_ip_bytes)
                        action = "answer"
                    elif qtype == QUERY_TYPE_AAAA:
                        response = build_response(packet, question)
                        action = "empty"
                    else:
                        response = build_response(packet, question)
                        action = "empty"
                else:
                    response = forward(packet, upstream)
                    action = "forward"

                sock.sendto(response, addr)
                print(
                    f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} "
                    f"{action} {name} qtype={qtype} client={addr[0]}:{addr[1]}",
                    flush=True,
                )
            except Exception as exc:
                try:
                    question = packet[12:]
                    sock.sendto(build_response(packet, question, rcode=2), addr)
                except Exception:
                    pass
                print(
                    f"{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())} error {exc}",
                    flush=True,
                )


if __name__ == "__main__":
    sys.exit(main())
