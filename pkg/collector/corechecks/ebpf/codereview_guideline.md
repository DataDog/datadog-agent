# eBPF core check review guidelines

## Capability selection and configuration

For kernel-dependent attachment choices, prefer runtime capability detection over adding a per-probe setting solely to select a supported implementation. Known kernel defects can justify an operator override; explain that exception and consider a shared system-probe control when it affects multiple probes.

## Program type is metadata

When attachment or lifecycle behavior depends on an eBPF program's kind, inspect its typed metadata, such as Program.Type(), rather than inferring the kind from its name. Flag name-based dispatch that can select the wrong operation after a rename or when taking ownership of an existing program.
