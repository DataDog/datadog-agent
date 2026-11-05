# Generic resource wire contract

Public producer envelope compatible with [integrations-core](https://github.com/DataDog/integrations-core/blob/0c777c85b49663c710e382bfa9a4eb051d570240/datadog_checks_base/datadog_checks/base/utils/genresources/proto/genericresource.proto); coordinate wire changes with both producers and REDAPL intake.
Regenerate Go bindings with `bazel run //pkg/proto/pbgo/genresources:write_pb_go`.
