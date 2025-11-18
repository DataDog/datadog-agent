""" Rule for building meson from source. """

load("@rules_python//python:defs.bzl", "py_binary")
load("@rules_python//python:features.bzl", "features")

def meson_tool(name, main, data, requirements = [], **kwargs):
    kwargs.pop("precompile", None)
    if not features.uses_builtin_rules:
        kwargs["precompile"] = "disabled"
    py_binary(
        name = name,
        srcs = [main],
        data = data,
        deps = requirements,
        python_version = "PY3",
        main = main,
        **kwargs
    )
