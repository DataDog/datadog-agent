# Golden host-benchmark snapshots

Informational per-rule result baselines, one file per framework named
`<framework>.txt` (e.g. `cis-rhel10.txt`); each line is `<rule-id> <result>`.

`TestGolden` logs the current snapshot and, when a matching baseline exists here,
logs the diff against it. It is **never fatal** — these files are a diagnostic
aid (to see exactly which rules changed when another test fails), not a gate.

They are tied to the pinned `SECURITY_AGENT_POLICIES_VERSION`. Regenerate from a
green CI run's `golden-<framework>.txt` artifact (or the `golden snapshot` log
line) and commit, e.g. after a policy-version bump.
