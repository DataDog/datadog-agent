# CI wiring

Load when adding a new test package or changing which build artifacts a test consumes. A test in a package no job references never runs.

Where the wiring lives:

| File | Holds |
|---|---|
| `.gitlab-ci.yml` and `.gitlab/test/e2e/*.yml` | Shared rules in the root file; team-specific rules beside their jobs |
| `.gitlab/test/e2e/*.yml` | Linux and cross-platform jobs, split by ownership |
| `.gitlab/windows/test/e2e/*.yml` and `.gitlab/windows/test/e2e_install_packages/windows.yml` | Windows jobs, split by ownership or package workflow |
| `.gitlab/JOBOWNERS` | Who is notified when the job fails |

## The rule template

Rule template names are abbreviated and do not track directory names — `tests/agent-runtimes` is gated by `.on_arun_or_e2e_changes`. Find the existing one rather than guessing:

```bash
grep -n '_or_e2e_changes:' .gitlab-ci.yml .gitlab/test/e2e/*.yml
```

A new one references the shared branch rule and adds the paths that should trigger it:

```yaml
.on_myarea_or_e2e_changes:
  - !reference [.on_e2e_main_release_or_rc]
  - changes:
      paths:
        - comp/myarea/**/*
        - pkg/myarea/**/*
        - test/new-e2e/tests/myarea/**/*
      compare_to: $COMPARE_TO_BRANCH
```

`.on_e2e_main_release_or_rc` is what makes the job run on `main`, release branches, and release candidates regardless of the diff. The `changes` block is what additionally runs it on a pull request that touches those paths. List the implementation paths, not only the test path — otherwise a change to the feature will not exercise its own test.

## The job

Extend a template that already declares the right artifact dependencies rather than hand-writing `needs`:

| Template | Brings in | Use for |
|---|---|---|
| `.new_e2e_template` | test binaries, tooling, fakeintake | Tests needing no agent package |
| `.new_e2e_template_needs_deb_x64` | `agent_deb-x64-a7`, `agent_deb-x64-a7-fips` | Host tests on Ubuntu or Debian |
| `.new_e2e_template_needs_container_deploy_linux` | `qa_agent_linux`, `qa_agent_linux_jmx`, `qa_dca`, `qa_dogstatsd` | Docker and Kubernetes on Linux |
| `.new_e2e_template_needs_container_deploy` | the above plus the Windows agent images | Container tests covering Windows |
| `.new_e2e_template_needs_windows_x64` | `windows_msi_and_bosh_zip_x64-a7` and its FIPS variant | Windows host tests (defined in `.gitlab/windows/test/e2e/windows_templates.yml`) |

```yaml
new-e2e-myarea:
  extends: .new_e2e_template_needs_deb_x64
  rules:
    - !reference [.on_myarea_or_e2e_changes]
    - !reference [.manual]
  variables:
    TARGETS: ./tests/myarea
    TEAM: myteam
    EXTRA_PARAMS: --skip "Windows"
    ON_NIGHTLY_FIPS: "true"
```

`TARGETS` is relative to `test/new-e2e/`. `TEAM` routes test results. `EXTRA_PARAMS` passes `--run` and `--skip` regexes, which is how a suite split across `_nix_test.go` and `_win_test.go` gets divided between jobs. `ON_NIGHTLY_FIPS` also runs the job in the nightly FIPS pipeline.

When none of the templates fits, compose from the shared reference so you inherit the base dependencies:

```yaml
  needs:
    - !reference [.needs_new_e2e_template]
    - agent_rpm-x64-a7
```

Other artifact jobs: `agent_rpm-x64-a7` (RPM distributions), `deploy_windows_testing-a7` and `deploy_windows_testing-a7-fips` (Windows MSI), `deploy_installer_oci` (Fleet Automation packages).

Ask for only the artifacts the test consumes. Without `needs`, GitLab waits for every earlier stage; with too many, the job blocks on builds it never uses and lengthens the pipeline for everyone. A test that hangs waiting for an image usually has a missing `needs`, not a broken test.

## Windows jobs

Each Windows installer test function provisions its own VM, so those jobs fan out with `parallel: matrix`, one entry per test function, and select with an anchored regex:

```yaml
  parallel:
    matrix:
      - E2E_MSI_TEST: TestInstall
      - E2E_MSI_TEST: TestUpgrade
  variables:
    TARGETS: ./tests/windows/install-test
    EXTRA_PARAMS: --run "$E2E_MSI_TEST$"
```

Adding a test function to one of those packages means adding a matrix entry, otherwise it never runs.

## Pre-initialising expensive infrastructure

A cluster that takes five to ten minutes to create can be built once by a separate job:

```yaml
new-e2e-myarea-init:
  extends: .new_e2e_template
  stage: e2e_init
  variables:
    TARGETS: ./tests/myarea
    E2E_INIT_ONLY: "true"

new-e2e-myarea:
  extends: .new_e2e_template
  needs:
    # extends replaces needs rather than merging, and the inherited before_script
    # unpacks these artifacts — dropping them fails the job on a missing tarball.
    - !reference [.needs_new_e2e_template]
    - new-e2e-myarea-init
  variables:
    TARGETS: ./tests/myarea
    E2E_PRE_INITIALIZED: "true"
```

`new-e2e-containers-eks` in `.gitlab/test/e2e/e2e_containers.yml` is the in-tree version; it re-references `.new_e2e_template_needs_container_deploy` alongside its init job for the same reason.

Worth it for EKS and similar; unnecessary for a single VM.

## Ownership

Add the job to `.gitlab/JOBOWNERS`, which is what dispatches a failure notification:

```
new-e2e-myarea*                   @DataDog/myteam
```

`new-e2e*` defaults to `@DataDog/agent-devx`, so a job without its own entry pages the framework team instead of the team that owns the behavior. Add the test directory to `.github/CODEOWNERS` as well — that governs review, not notifications, and the two are separate on purpose.

## Budgets and branch coverage

`test/new-e2e/codereview_guideline.md` § "Keeping tests fast" sets the wall-time budgets. A job gated on every pull request regardless of paths is rare by design and needs justifying.

Most E2E jobs run only on `main`, release branches, and release candidates. A change whose only coverage is such a job gets no pull-request signal — say so in the report, since a reviewer cannot tell from a green pipeline. That is a disclosure, not a label: a test-only pull request still takes `qa/no-code-change`, and `qa/rc-required` is reserved for changes that genuinely can only be validated on a release candidate.

## Dynamic test skipping

Some e2e jobs prune themselves from the inside. A job whose `rules` reference `.dynamic_tests` (`.gitlab-ci.yml`) is created on any change under `pkg/`, `cmd/`, or `comp/` — deliberately broad — and then the `--impacted` flag on `new-e2e-tests.run` consults a coverage index and skips the tests in that job the diff does not touch. It selects tests within a job, never between jobs, so it neither replaces nor relaxes the `changes` rule above.

Three consequences for a test author:

- A test the index does not know about is never skipped. The skip list is `indexed tests − impacted tests` (`tasks/libs/dynamic_test/index.py`), so a newly added test always runs.
- Pruning happens on dev branches only. `main`, release branches, tagged commits, and triggered pipelines run everything, as does setting `RUN_E2E_TESTS=on` or the breakglass secret.
- A failure to load the index is logged and the run falls back to the full suite, so a missing index costs time rather than coverage.

Ask in `#agent-devx-help` when a job needs an artifact or a cloud capability that does not exist yet.
