# AIX Changelog

<!-- This file tracks changes to the Datadog Agent on AIX.
     It is maintained manually until AIX is officially supported and released
     through the same process as other platforms, at which point it will be
     replaced by the standard release notes mechanism. Each PR that affects
     AIX should add an entry to the current (unreleased) section below. -->

## Unreleased

- Initial AIX packaging pipeline supporting AIX 7.3 (ppc64)
- Agent binary, trace-agent, and process-agent built from source on AIX 7.3
- Embedded Python 3.13 with pip, supporting Go and Python checks
- Bundled integrations: `lparstats` (LPAR performance metrics)
- IBM integrations available: `ibm_ace`, `ibm_db2`, `ibm_i`, `ibm_mq`, `ibm_spectrum_lsf`, `ibm_was`
- installp (BFF) packaging with pre/post install scripts, service management via SRC
- Static linking of libstdc++ in rtloader for compatibility with AIX 7.2+
- jellyfish Python package built from source using the IBM Rust SDK
