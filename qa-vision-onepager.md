# QA experience on datadog-agent — what I'd like to do

*(One-pager for the daily — full proposal: "My vision regarding testing and qa experience
on datadog-agent" on Confluence.)*

**The idea in three sentences.** Today E2E environments are ephemeral and CI-owned. I want
to flip that: a CLI (`e2ectl`) plus **one YAML config language**, where an environment is a
**durable asset** — you create it once, install the agent on it, iterate against it, and
point tests at it. Locally and in the CI: the same tool, two modes; shared "classics"
(eks-1-29, kind-latest, docker-ubuntu22…) provide defaults, and every test carries one
`config.yaml` describing what it needs and what it runs on.

## Scenario 1 — I'm working on a Kubernetes feature

My whole QA setup lives in one small YAML file:

```yaml
# ksm-dev.yml
environment:
  base: kind-latest          # from the classics catalog
  fakeintake: true
agent:
  source: local              # build from my working tree
  install: helm
workloads:
  - manifest: my-app.yaml    # my app, auto-wired to talk to the agent
```

Three commands and it's up:

```
e2ectl start   --config ksm-dev.yml --name ksm-dev   # cluster + fakeintake, kubectl wired
e2ectl install --config ksm-dev.yml --env ksm-dev    # agent with my code, my API key
e2ectl deploy  --config ksm-dev.yml --env ksm-dev    # my app, configured to talk to it
```

Then the loop where I spend my day:

```
e2ectl fakeintake metrics --name 'kube*' --env ksm-dev --json
# ... edit code ...
e2ectl update --env ksm-dev     # rebuilds only what changed, same cluster — ~2 min/cycle
```

And when I'm ready: `e2ectl test --suite ./test/new-e2e/tests/containers/... --env ksm-dev`
— existing suites run against my live cluster, no provisioning.

## Scenario 2 — I'm working on a core agent feature

Same verbs, one different file — a VM instead of a cluster, **binary install instead of
.deb** (for core work I only care about the binary anyway):

```yaml
# core-dev.yml
environment:
  base: ec2-ubuntu-latest    # from the classics catalog
  fakeintake: true
agent:
  source: local
  install: binary
workloads:
  - manifest: my-app.yaml
```

`update` rebuilds the changed binaries, copies them over, restarts the agent — **under a
minute per cycle**. Same fakeintake inspection, same `e2ectl test` against the host
suites.

## Why this is worth it

- **A failed test no longer destroys the crime scene.** Locally the env stays; for a CI
  failure, `e2ectl start --config <that test's config.yaml>` re-creates exactly what the
  test ran on — one command, no translation.
- **Tests declare their world in config, not in Go plumbing** — no provisioner args, no
  agent params structs; CI jobs are generated from the same configs.
- **Same tool in the CI**: create-from-config there, verify-and-reuse locally.

**Two questions for you:** would this replace your current loop? And which step breaks
for your team's use case?
