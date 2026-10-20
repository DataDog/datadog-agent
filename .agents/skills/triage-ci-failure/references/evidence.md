# Evidence reference — reading a job log without dumping it into context

Load this when working through Step 4 of `SKILL.md`.

A failed `new-e2e-*` trace is easily 50,000+ lines. Work through these in order;
each one is cheap, and most triage runs never need to go past step 3.

```bash
# 1. Section map — the cheapest possible structural overview.
ddgl logs --job <id> --no-pager --no-color | grep -n '^───'

# 2. The end, where the failure actually surfaces.
ddgl logs --job <id> --raw --no-pager | tail -200

# 3. Error lines, in case the interesting one scrolled past the tail.
ddgl logs --job <id> --raw --no-pager \
  | grep -nEi '(error|fatal|panic|FAIL|Traceback|exit code|assertion)' | tail -60

# 4. Go test failures specifically.
ddgl logs --job <id> --raw --no-pager | grep -nE '^\s*--- FAIL:|^FAIL\s' | head -40

# 5. A targeted read once you have a line number from any of the above.
ddgl logs --job <id> --raw --no-pager | sed -n '<n>,<n+80>p'
```

If you're exporting logs for several jobs at once instead of reading one at a
time, `mkdir -p` the output directory first — `ddgl logs --output <dir>` silently
concatenates every job's log into a single file (rather than one file per job) if
the directory doesn't already exist:

```bash
mkdir -p failures && ddgl logs --pipeline <id> --failed --output failures/
```

## What to look for

Deliberately general rather than a pattern list, because pattern lists rot the
moment CI infrastructure changes underneath them:

- **The command that actually failed, and its exit status.** Most job scripts run
  several steps; the trace usually says which one exited non-zero.
- **Whether it failed in the job's own work, or in setup/teardown.** A failure
  before the job's real script even starts is a much stronger infra signal than
  one inside the test itself.
- **Error text that names something concrete** — an image reference, a hostname,
  an endpoint, a bucket, a package version. That string is exactly what feeds tier
  3 of the incident search (`references/signals.md`) if you end up needing it.
