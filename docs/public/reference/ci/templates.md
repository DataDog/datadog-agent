# CI templates

---

The `ci/templates` directory contains templates for use in GitLab CI.

## Global

Every `.yml` file at the root of the `ci/templates` directory is meant to be included in all pipelines as follows.

```yaml
include:
- local: ci/templates/*.yml
```

This allows for easy job creation via `extends` composition. For example, the following defines a job that runs on Linux ARM64 and uses the Linux build image.

```yaml
.job:linux:arm64:
  extends:
  - .runner:linux:arm64
  - .image:build:linux
  script:
  - echo "Hello, world!"
```

## Explicit

All non-global templates are meant to be included only in the pipelines that need them. Some may be used to compose jobs via `extends` while others may include jobs directly via parameterized `inputs`. The following is an example of using parameterization to override the default message for the job when a unit is skipped.

```yaml
include:
- local: ci/templates/jobs/units/skip.yml
  inputs:
    message: "Unit does not need to run: $UNIT_DISPLAY_NAME"
```
