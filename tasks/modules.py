from __future__ import annotations

import json
import os
import sys
from collections import defaultdict
from contextlib import contextmanager
from pathlib import Path

import yaml
from invoke import Context, Exit, task

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.gomodules import (
    IGNORED_MODULE_PATHS,
    GoModule,
    GoModuleDumper,
    get_default_modules,
    list_default_modules,
)

AGENT_MODULE_PATH_PREFIX = "github.com/DataDog/datadog-agent/"


MAIN_TEMPLATE = """package main

import (
{imports}
)

func main() {{}}
"""

PACKAGE_TEMPLATE = '	_ "{}"'


@contextmanager
def generate_dummy_package(ctx, folder):
    """
    Return a generator-iterator when called.
    Allows us to wrap this function with a "with" statement to delete the created dummy pacakage afterwards.
    """
    try:
        import_paths = []
        for mod in get_default_modules().values():
            if mod.path != "." and mod.verify_condition() and mod.importable:
                import_paths.append(mod.import_path)

        os.mkdir(folder)
        with ctx.cd(folder):
            print("Creating dummy 'main.go' file... ", end="")
            with open(os.path.join(ctx.cwd, 'main.go'), 'w') as main_file:
                main_file.write(
                    MAIN_TEMPLATE.format(imports="\n".join(PACKAGE_TEMPLATE.format(path) for path in import_paths))
                )
            print("Done")

            ctx.run("go mod init example.com/testmodule")
            for mod in get_default_modules().values():
                if mod.path != ".":
                    ctx.run(f"go mod edit -require={mod.dependency_path('0.0.0')}")
                    ctx.run(f"go mod edit -replace {mod.import_path}=../{mod.path}")
                    # todo: remove once datadogconnector fix is released.
                    if mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl":
                        ctx.run(
                            "go mod edit -replace github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector=github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector@v0.103.0"
                        )
                    if (
                        mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/configstore/impl"
                        or mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
                    ):
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/maps@v0.1.1")
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/providers/confmap@v0.1.0")
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/providers/confmap@v0.1.0-dev0")
        # yield folder waiting for a "with" block to be executed (https://docs.python.org/3/library/contextlib.html)
        yield folder

    # the generator is then resumed here after the "with" block is exited
    finally:
        # delete test_folder to avoid FileExistsError while running this task again
        ctx.run(f"rm -rf ./{folder}")


@task
def go_work(_: Context):
    """
    Create a go.work file using the module list contained in get_default_modules()
    and the go version contained in the file .go-version.
    If there is already a go.work file, it is renamed go.work.backup and a warning is printed.
    """
    print(
        color_message(
            "WARNING: Using a go.work file is not supported and can cause weird errors "
            "when compiling the agent or running tests.\n"
            "Remember to export GOWORK=off to avoid these issues.\n",
            "orange",
        ),
        file=sys.stderr,
    )

    # read go version from the .go-version file, removing the bugfix part of the version

    with open(".go-version") as f:
        go_version = f.read().strip()

    if os.path.exists("go.work"):
        print("go.work already exists. Renaming to go.work.backup")
        os.rename("go.work", "go.work.backup")

    with open("go.work", "w") as f:
        f.write(f"go {go_version}\n\nuse (\n")
        for mod in get_default_modules().values():
            prefix = "" if mod.verify_condition() else "//"
            f.write(f"\t{prefix}{mod.path}\n")
        f.write(")\n")


@task
def for_each(
    ctx: Context,
    cmd: str,
    skip_untagged: bool = False,
    ignore_errors: bool = False,
    use_targets_path: bool = False,
    use_lint_targets_path: bool = False,
    skip_condition: bool = False,
):
    """
    Run the given command in the directory of each module.
    """
    assert not (
        use_targets_path and use_lint_targets_path
    ), "Only one of use_targets_path and use_lint_targets_path can be set"

    for mod in get_default_modules().values():
        if skip_untagged and not mod.should_tag:
            continue
        if skip_condition and not mod.verify_condition():
            continue

        targets = [mod.full_path()]
        if use_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.targets]
        if use_lint_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.lint_targets]

        for target in targets:
            with ctx.cd(target):
                res = ctx.run(cmd, warn=True)
                assert res is not None
                if res.failed and not ignore_errors:
                    raise Exit(f"Command failed in {target}")


@task
def validate(_: Context):
    """
    Test if every module was properly added in the get_default_modules() list.
    """
    missing_modules: list[str] = []
    default_modules_paths = {Path(p) for p in get_default_modules()}

    # Find all go.mod files and make sure they are registered in get_default_modules()
    for root, dirs, files in os.walk("."):
        dirs[:] = [d for d in dirs if Path(root) / d not in IGNORED_MODULE_PATHS]

        if "go.mod" in files and Path(root) not in default_modules_paths:
            missing_modules.append(root)

    if missing_modules:
        message = f"{color_message('ERROR', Color.RED)}: some modules are missing from get_default_modules()\n"
        for module in missing_modules:
            message += f"  {module} is missing from get_default_modules()\n"

        message += "Please add them to the get_default_modules() list or exclude them from the validation."

        raise Exit(message)


@task
def validate_used_by_otel(ctx: Context):
    """
    Verify whether indirect local dependencies of modules labeled "used_by_otel" are also marked with the "used_by_otel" tag.
    """
    otel_mods = [path for path, module in get_default_modules().items() if module.used_by_otel]
    missing_used_by_otel_label: dict[str, list[str]] = defaultdict(list)

    # for every module labeled as "used_by_otel"
    for otel_mod in otel_mods:
        gomod_path = f"{otel_mod}/go.mod"
        # get the go.mod data
        result = ctx.run(f"go mod edit -json {gomod_path}", hide='both')
        if result.failed:
            raise Exit(f"Error running go mod edit -json on {gomod_path}: {result.stderr}")

        go_mod_json = json.loads(result.stdout)
        # get module dependencies
        reqs = go_mod_json.get("Require", [])
        if not reqs:  # Module don't have dependencies, continue
            continue
        for require in reqs:
            # we are only interested into local modules
            if not require["Path"].startswith("github.com/DataDog/datadog-agent/"):
                continue
            # we need the relative path of module (without github.com/DataDog/datadog-agent/ prefix)
            rel_path = require['Path'].removeprefix("github.com/DataDog/datadog-agent/")
            # check if indirect module is labeled as "used_by_otel"
            if rel_path not in get_default_modules() or not get_default_modules()[rel_path].used_by_otel:
                missing_used_by_otel_label[rel_path].append(otel_mod)
    if missing_used_by_otel_label:
        message = f"{color_message('ERROR', Color.RED)}: some indirect local dependencies of modules labeled \"used_by_otel\" are not correctly labeled in get_default_modules()\n"
        for k, v in missing_used_by_otel_label.items():
            message += f"\t{color_message(k, Color.RED)} is missing (used by {v})\n"
        message += "Please label them as \"used_by_otel\" in the get_default_modules() list."

        raise Exit(message)


def get_module_by_path(path: Path) -> GoModule | None:
    """
    Return the GoModule object corresponding to the given path.
    """
    for module in get_default_modules().values():
        if Path(module.path) == path:
            return module

    return None


def _print_modules(modules: dict[str, GoModule], details: bool, remove_defaults: bool):
    """Print the module mapping to stdout.

    Args:
        details: If True, will show also the contents of each module (will list only the names otherwise).
        remove_defaults: If True, will remove default values from the output.
    """

    if not details:
        print("\n".join(sorted(modules.keys())))
        return

    modules_data = {path: module.to_dict(remove_defaults=remove_defaults) for path, module in modules.items()}
    for module in modules_data.values():
        del module["path"]

    yaml.dump(modules_data, sys.stdout, Dumper=GoModuleDumper)


@task
def show(_, path: str, remove_defaults: bool = False, base_dir: str = '.'):
    """Show the module information for the given path."""

    default_modules, ignored_modules = list_default_modules(Path(base_dir))
    if path in ignored_modules:
        print(f'Module {path} is ignored')
        return

    module = default_modules.get(path)

    assert module, f'Module {path} not found'

    _print_modules({path: module}, details=True, remove_defaults=remove_defaults)


@task
def show_all(_, details: bool = False, remove_defaults: bool = True, base_dir: str = '.', ignored=False):
    """Show the list of modules.

    Args:
        details: If True, will show also the contents of each module (will list only the names otherwise).
        remove_defaults: If True, will remove default values from the output.
        ignored: If True, will list ignored modules.
    """

    if ignored:
        _, ignored_modules = list_default_modules(Path(base_dir))
        print('\n'.join(sorted(ignored_modules)))
    else:
        _print_modules(get_default_modules(base_dir), details=details, remove_defaults=remove_defaults)
