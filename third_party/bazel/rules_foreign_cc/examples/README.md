# Rules Foreign CC Examples

## Top-Level

Top-level examples should contain no dependencies outside of `rules_foreign_cc` directly and anything else in
it's directory. The directories should be prefixed with the type of rule they're associated with. Eg
`cmake_with_data` (being a [cmake_external][cmake_external] example) and `configure_with_bazel_transitive`
(being a [configure_make][configure_make] example).

## Third Party

Examples of building source from outside of `rules_foreign_cc` should be put in the `third_party` directory
which is an isolated workspace that gets added to `rules_foreign_cc_examples` as an additional
`rules_foreign_cc_examples_third_party` repository. In general, these are expected to be expensive to build
so adding new things here should be done selectively. In the top-level package of this workspace, there are
test suites separated by the operating system that all tests need to be registered with. The expected structure of
any example in this workspace are as follows:

```text
third_party/lib
├── BUILD.bazel
├── BUILD.lib.bazel
└── lib_repositories.bzl
```

### BUILD.bazel

This file must contain some sort of test that confirms the targets for the external repo can be successfully
built and ideally ran. The targets here will need to be registered in the [test_suite's][test_suite] found in
[`./third_party/BUILD.bazel`](./third_party/BUILD.bazel) or they will not be ran in CI.

### BUILD.lib.bazel

The BUILD file expected to be used in the repository containing the target source code.

### lib_repositories.bzl

A file containing a single macro `lib_repositories` that should define the desired repository and ensure the
`BUILD.lib.bazel` file will correctly be installed when the repository is setup. This macro will need to be
loaded and called in [`third_party/repositories.bzl`](./third_party/repositories.bzl).

[cmake_external]: https://github.com/bazel-contrib/rules_foreign_cc/tree/main/docs#cmake_external
[configure_make]: https://github.com/bazel-contrib/rules_foreign_cc/tree/main/docs#configure_make
[test_suite]: https://docs.bazel.build/versions/master/be/general.html#test_suite
