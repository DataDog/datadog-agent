#include <stdio.h>

#include <openssl/bio.h>
#include <openssl/crypto.h>
#include <openssl/ssl.h>

int main(void) {
    SSL_CTX *ctx = SSL_CTX_new(TLS_client_method());
    if (ctx == NULL) {
        return 1;
    }
    SSL_CTX_free(ctx);

    // libcrypto writes to this binary's stdout, which requires both sides to
    // share the C runtime.
    BIO *out = BIO_new_fp(stdout, BIO_NOCLOSE);
    if (out == NULL) {
        return 1;
    }
    BIO_printf(out, "%s\n", OpenSSL_version(OPENSSL_VERSION));
    BIO_free(out);
    return 0;
}
