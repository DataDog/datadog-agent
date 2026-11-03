"""Check that the built interpreter rejects a chain through a compromised eMudhra key.

Run by distrusted_spki_test under the built interpreter, not the test one.
The fixtures come from CPython's own gh-160040 test: a localhost leaf signed
by EM OV TLS CA - G3C, whose leaked key is on the _ssl denylist, chaining to
emSign Root TLS CA - G3. Without the patch the handshake succeeds.

Usage: python -I distrusted_spki_check.py <distrusted.zip>
"""

import os
import ssl
import sys
import tempfile
import zipfile

# CPython encrypts the archive only to keep secret scanners quiet; the
# password is public.
_ZIP_PASSWORD = b"Brave Sir Robin"
_CERT_REJECTED = 28


def _handshake(client_ctx, server_ctx):
    c_in, c_out, s_in, s_out = (ssl.MemoryBIO() for _ in range(4))
    client = client_ctx.wrap_bio(c_in, c_out, server_hostname="localhost")
    server = server_ctx.wrap_bio(s_in, s_out, server_side=True)
    pending = [client, server]
    while pending:
        for side in list(pending):
            try:
                side.do_handshake()
                pending.remove(side)
            except ssl.SSLWantReadError:
                pass
        s_in.write(c_out.read())
        c_in.write(s_out.read())


def main(zip_path):
    client_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    with tempfile.TemporaryDirectory() as tmp:
        with zipfile.ZipFile(zip_path) as archive:
            archive.extractall(tmp, pwd=_ZIP_PASSWORD)
        certs = os.path.join(tmp, "distrusted")
        client_ctx.load_verify_locations(os.path.join(certs, "distrusted_root.pem"))
        server_ctx.load_cert_chain(os.path.join(certs, "distrusted_keycert.pem"))

    try:
        _handshake(client_ctx, server_ctx)
    except ssl.SSLCertVerificationError as exc:
        if exc.verify_code == _CERT_REJECTED:
            print(f"rejected: {exc.verify_message}")
            return 0
        print(f"failed for another reason: {exc}")
        return 1
    print("handshake succeeded through a distrusted key")
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1]))
