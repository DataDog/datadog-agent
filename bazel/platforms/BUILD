package(default_visibility = ["//visibility:public"])

constraint_setting(
    name = "glibc_version",
    default_constraint_value = ":glibc_any",
)

constraint_value(
    name = "glibc_any",
    constraint_setting = ":glibc_version",
)

constraint_value(
    name = "glibc_2_17",
    constraint_setting = ":glibc_version",
)

constraint_value(
    name = "glibc_2_23",
    constraint_setting = ":glibc_version",
)

platform(
    name = "linux_x64",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:x86_64",
        ":glibc_2_17",
    ],
    visibility = ["//visibility:public"],
)

platform(
    name = "linux_arm64",
    constraint_values = [
        "@platforms//os:linux",
        "@platforms//cpu:arm64",
        ":glibc_2_23",
    ],
    visibility = ["//visibility:public"],
)
