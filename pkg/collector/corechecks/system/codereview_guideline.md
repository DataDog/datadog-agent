# System core check review guidelines

## Validate deterministic configuration before collection

Flag validation that depends only on supplied configuration but is deferred until every collection run. Perform it during parsing or Configure so invalid instances fail with actionable configuration errors. For the PowerShell check, this includes parameter names, case-insensitive duplicate parameters, and projected property identifiers. Preserve documented fallback behavior. Checks that require live external state still belong in the runtime path.
