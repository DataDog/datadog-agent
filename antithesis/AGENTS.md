This directory contains files relevant to running tests in Antithesis, targeting the **Datadog Cluster Agent**.

**Start with `README.md`** for a concise, current summary of what's built, what's verified working, and what's left — read that before diving into `scratchbook/`.

Use the `antithesis-research` skill to analyze the system and build a property catalog. Use the `antithesis-setup` skill to scaffold and manage this directory. Use the `antithesis-workload` skill to implement assertions and test commands. Use the `antithesis-launch` skill to build, validate, and submit Antithesis runs — do not run `snouty launch` directly.

**snouty launch**
Use `snouty launch --json --webhook basic_test --config antithesis/config` to start an Antithesis run. Always run `compose build` first to ensure images are up to date.

**snouty validate**
Use this command to quickly validate changes to the Antithesis scaffolding. See `snouty validate --help` for details.

**setup-complete.sh**
Inject this script into a Dockerfile to notify Antithesis that setup is complete. This script should only run once the system under test is ready for testing. Antithesis will not run any test commands until it receives this event.

**config**
This directory contains the `docker-compose.yaml` file used to bring up this system within the Antithesis environment, along with any closely related config files. Snouty will push tagged images, consume this config directory, and launch the run.

**scratchbook**
This directory is the Antithesis scratchbook for the codebase. It contains system analysis (`sut-analysis.md`), the property catalog (`property-catalog.md`, 37 properties), deployment topology (`deployment-topology.md`), property relationship maps, per-property evidence files (`scratchbook/properties/`), and evaluation records (`scratchbook/evaluation/`). Keep it up to date as Antithesis-related decisions change.

**test**
This directory contains test templates. A test template is a directory containing test command executable files. Each test command must have a valid prefix: `parallel_driver_, singleton_driver_, serial_driver_, first_, eventually_, finally_, anytime_`. Prefixes constrain when and how commands are composed in a single timeline. Files or subdirectories prefixed with `helper_` are ignored by Antithesis and can be used for helper scripts kept alongside the commands.
