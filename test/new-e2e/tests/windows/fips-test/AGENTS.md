# Windows FIPS Agent MSI Tests

Tests for the Windows FIPS Agent MSI installer. Covers installation of the
FIPS Agent in standard and alternate directories, and mutual exclusivity
between the standard Agent and the FIPS Agent (each should refuse to install
over the other).

These tests use the FIPS-flavor MSI (`WINDOWS_AGENT_FLAVOR=fips`) alongside
the standard MSI. Both are resolved from environment variables using the same
`CURRENT_AGENT_*` / `STABLE_AGENT_*` pattern as the other MSI tests (see
parent `AGENTS.md`).

## What is tested

Beyond install/uninstall correctness, the FIPS tests validate the OpenSSL
setup that the MSI is responsible for:

- `fips.dll` (the OpenSSL FIPS provider) is present under `embedded3/lib/ossl-modules/`
- `fipsmodule.cnf` is present and valid — verified by running
  `openssl.exe fipsinstall -verify` against it
- The MSI writes the correct OpenSSL directory paths (`OPENSSLDIR`, `ENGINESDIR`,
  `MODULESDIR`) into the registry under
  `HKLM:\SOFTWARE\Wow6432Node\OpenSSL-<version>-datadog-fips-agent`, and
  those paths exist on disk
- `openssl.exe version -a` reports paths that match the registry values and
  is compiled with `-DOSSL_WINCTX=datadog-fips-agent`

## Related tests

FIPS cipher and compliance tests for Windows live in a separate package:
`test/new-e2e/tests/fips-compliance/`. That package contains:

- **`TestWindowsVM`** (`fips_win_test.go`) — validates the FIPS Agent runs
  correctly on a Windows VM
- **`TestFIPSCiphersWindowsSuite`** (`fips_ciphers_win_test.go`) — validates
  that the agent only negotiates FIPS-approved cipher suites

## CI

Jobs are in `.gitlab/windows/test/e2e_install_packages/windows.yml` under
`new-e2e-windows-agent-a7-x86_64-fips`. The compliance tests are in
`.gitlab/windows/test/e2e/windows.yml` under
`new-e2e-windows-fips-compliance-test`.
