# eBPF core check review guidelines

## Capability selection and configuration

For kernel-dependent attachment choices, prefer automatic runtime capability detection. Allow an explicit configuration override as a documented escape hatch for known kernel defects. If the same defect affects several probes, consider one system-probe setting rather than a separate setting for each probe.

## Program type is metadata

When attachment or lifecycle behavior depends on an eBPF program's kind, inspect its typed metadata, such as Program.Type(), rather than inferring the kind from its name. Flag name-based dispatch that can select the wrong operation after a rename or when taking ownership of an existing program.
