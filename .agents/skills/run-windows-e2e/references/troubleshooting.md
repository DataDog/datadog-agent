# Troubleshooting — Windows E2E Tests

## Test outputs

Test output files (logs, crash dumps, event logs, agent logs) are written via
`runner.GetProfile().CreateOutputSubDir()` to:

```
~/e2e-output/<suite-name>/<timestamp>/
~/e2e-output/latest/                    ← symlink to the most recent run
```

When a test fails, the suite's `AfterTest` hook automatically collects
diagnostics into the output directory — WER crash dumps, Windows event logs
(`.evtx`), agent logs, and installer logs depending on the suite. Check
`~/e2e-output/latest/` first when investigating a failure. If `devMode` is on,
the VM is still up — connect to it via [vm-access.md](vm-access.md).

In CI, outputs are stored as GitLab job artifacts, accessible from the job's
artifact browser in the pipeline UI.

## Pulumi lock errors

If you see `error: the stack is currently locked by 1 lock(s)`:

```bash
dda inv new-e2e-tests.clean            # remove local Pulumi locks
dda inv new-e2e-tests.clean -s         # also clean local stack state
dda inv new-e2e-tests.clean --output   # clear local test output
```

If cleanup reports "Cleanup supported for local state only":

```bash
pulumi login --local
```

## AWS account / profile

Local runs must use the `agent-sandbox` AWS account (CI uses `agent-qa`). The
e2e framework automatically uses the `sso-agent-sandbox-account-admin` profile,
so no explicit `aws-vault` invocation is needed.

However, if `AWS_PROFILE` is already set to a different profile it overrides the
automatic selection and causes auth errors. Unset it before running:

```bash
unset AWS_PROFILE
```

A common symptom of the wrong account:
`User: arn:aws:sts::... is not authorized to perform: ecr:BatchGetImage`.
