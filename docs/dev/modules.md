# Creating a Go nested module

## Go nested modules tooling

Go nested modules interdependencies are automatically updated when creating a release candidate or a final version, with the same tasks that update the `release.json`. For Agent version `7.X.Y` the module will have version `v0.X.Y`.

Go nested modules are tagged automatically by the `release.tag-version` invoke task, on the same commit as the main module, with a tag of the form `path/to/module/v0.X.Y`.

## The `modules.yml` file

The `modules.yml` file gathers all go modules configuration.
Each module is listed even if this module has default attributes or is ignored.

Here is an example:

```yaml
modules:
  .:
    independent: false
    lint_targets:
    - ./pkg
    - ./cmd
    - ./comp
    test_targets:
    - ./pkg
    - ./cmd
    - ./comp
  comp/api/api/def:
    used_by_otel: true
  comp/api/authtoken: default
  test/integration/serverless/src: ignored
  tools/retry_file_dump:
    should_test_condition: never
    independent: false
    should_tag: false
```

`default` is for modules with default attribute values and `ignored` for ignored modules.
To create a special configuration, the attributes of `GoModule` can be overriden. Attributes details are located within the `GoModule` class.

This file is linted with `dda inv modules.validate [--fix-format]`.
