# DatadogInstrumentation shared contract

This package contains the product-agnostic `Handler` contract, event/status types,
GVR, and object conversion helpers used by reconciliation and admission. Controller
and product handler implementations live in `comp/instrumentation/`; see its
`AGENTS.md` for architecture, extension, startup, and testing guidance.

Keep this package independent of product handlers and their stores. Files require
`kubeapiserver`; the CRD schema is defined in the datadog-operator API module.
