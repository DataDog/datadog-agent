# CI structure

---

/// note
This document only discusses our GitLab CI infrastructure.
///

## Units

The CI is structured into discrete units to accommodate the needs of all projects as we transition to a monorepo.

A CI unit is a collection of jobs that run independently of other units. All [units](units/defined.md) are defined by TOML files in the `/ci/units` directory, each being solely owned by the team responsible for the unit.

Units determine their own trigger conditions, whether execution is defined statically or dynamically, and other [configuration](units/config.md). Concrete CI provider-specific files are generated in the `/.ci/units` directory for each unit with the following command.

```
dda check ci units --fix
```

/// tip
See the tutorial for [managing CI units](../../tutorials/ci/units.md).
///

## Entry point

The entry point for the CI is an intentionally minimal `/.gitlab-ci.yml` file at the root of the repository. It contains the following:

- Variable definitions used in manually-triggered pipelines, such as for unit selection
- Local includes for templates, global variables and the generated units
- Declarations of only two custom stages: one for triggering units and another for generating pipeline files for dynamic units
- A job (in the `.pre` stage) to validate the units that must pass before any other jobs can run

After validation, static units are immediately triggered while dynamic units wait for their associated generation job to complete.
