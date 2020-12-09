#!/usr/bin/env python

import sys

NX_DOMAIN_HEADER = b'\x81\x83\x00\x01\x00\x00\x00\x00\x00\x00'
request_id = sys.stdin.read(2)

# Consume HEADER(2), QDCOUNT(2), ANCOUNT(2), NSCOUNT(2), ARCOUNT(2)
sys.stdin.read(10)

# Now we read the query section (which we'll include in the answer)
query = b''
while True:
    b = sys.stdin.read(1)
    query += b

    # When we reach the null byte it means we have fully read the QNAME field
    if b == b'\x00':
        # Read QTYPE(2) and QCLASS(2)
        query += sys.stdin.read(4)
        break

sys.stdout.write(request_id)
sys.stdout.write(NX_DOMAIN_HEADER)
sys.stdout.write(query)
