# Fleet upgrade history

`TestFleetUpgradeHistory` tests a direct upgrade from every released Agent patch
in the four minor releases preceding the pipeline candidate. For a `7.87`
candidate, the source minors are `7.86`, `7.85`, `7.84`, and `7.83`, even when
the latest stable version belongs to a different minor.

The source catalog includes every numeric package revision, deduplicates registry
aliases, and excludes release candidates and development builds. A minor with no
published stable packages is reported as a skipped subtest (main can advance
before the preceding minor is released); it is never replaced with another minor.
Catalog retrieval errors and failures to install a published package fail the test.

Every source/platform pair provisions a separate host. Linux installs the exact
historical DEB/RPM; Windows uses the matching historical installer executable.
The test checks the installed source package and running Agent, starts an
experiment with the pipeline candidate, verifies its running package version,
and promotes it. Promotion must leave the candidate stable with no experiment.

`new-e2e-fleet-upgrade-history` runs on the same installer/E2E change rules as
the other Fleet jobs. Its eight shards cover Linux/Windows and minor offsets
1–4, using the existing Fleet platform lists. Dynamic test skipping and early
flake detection are disabled for this job. Each job limits parallel tests to four
and destroys each completed suite's VM before reusing its test slot.

To select one minor and platform family locally, after the standard E2E setup:

```sh
E2E_FLEET_PLATFORM_GROUP=linux E2E_FLEET_UPGRADE_MINOR_OFFSET=2 \
  dda inv new-e2e-tests.run --targets=./tests/fleet \
  --pipeline-id=<candidate-pipeline-id> \
  --run '^TestFleetUpgradeHistory$' --extra-flags=-test.parallel=4
```

Without `E2E_FLEET_UPGRADE_MINOR_OFFSET`, the suite covers all four preceding
minors. Source versions and the exact candidate are recorded in the test output.
