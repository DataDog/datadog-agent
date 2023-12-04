# Deprecated Architectures

## 32-bit Windows

There was a product request to release the Agent on Windows 32 bits but it never came to fruition.

Datadog removed all 32-bit Windows related code in [#21221](https://github.com/DataDog/datadog-agent/pull/21221). The PR:

- Removes the `*_386.go` files.
- Removes the windows 32 bits logic in `omnibus/config`.
- Removes the Golang install for 32-bit Windows in the `devenv/scripts/Install-DevEnv.ps1` script.

Datadog also removed the tooling arguments for invoke tasks in [#21240](https://github.com/DataDog/datadog-agent/pull/21240).
