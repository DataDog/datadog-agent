This directory contains java key and trust stores used for mutual TLS tests.

There two sets of keystores, split by keystore format used:

- `pkcs12` used in by non-FIPS tests using standard openjdk crypto providers.
- `bcfks` used by the FIPS tests using BouncyCastle crypto provider and its proprietary keystore format.

Each keystore contains one unique private key and certificate for use by either jmxfetch or the test
app.

Trust stores will accept matching certificates from either set (`pkcs12` jmxfetch will trust app
keys from both `pkcs12` and `bcfks`, `bcfks` app will trust jmxfetch keys from both `pkcs12` and
`bcfks`, and so on).

See `generate.sh` for exact process of creating these.

