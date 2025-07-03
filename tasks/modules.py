from __future__ import annotations

import json
import os
import re
import sys
import tempfile
from collections import defaultdict
from contextlib import contextmanager
from glob import glob
from pathlib import Path

import yaml
from invoke import Context, Exit, task

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.gomodules import (
    ConfigDumper,
    Configuration,
    GoModule,
    get_default_modules,
    validate_module,
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
            if mod.path != "." and mod.should_test():
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
def go_work(ctx: Context):
    """
    Update the go work to use all the modules defined in modules.yml
    """

    ctx.run(
        "go run ./internal/tools/worksynchronizer/worksynchronizer.go --path ./go.work --modules-file ./modules.yml"
    )


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
        if skip_condition and not mod.should_test():
            continue

        targets = [mod.full_path()]
        if use_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.test_targets]
        if use_lint_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.lint_targets]

        for target in targets:
            with ctx.cd(target):
                res = ctx.run(cmd, warn=True)
                assert res is not None
                if res.failed and not ignore_errors:
                    raise Exit(f"Command failed in {target}")


@task
def validate(ctx: Context, base_dir='.', fix_format=False):
    """
    Lints module configuration file.

    Args:
        fix_format: If True, will fix the format of the configuration files.
    """

    base_dir = Path(base_dir)
    config = Configuration.from_file(base_dir)
    default_attributes = GoModule.get_default_attributes()

    # Verify format
    with tempfile.TemporaryDirectory() as tmpdir:
        config.base_dir = Path(tmpdir)
        config.to_file()
        config.base_dir = base_dir

        if not ctx.run(
            f'diff -u {base_dir / Configuration.FILE_NAME} {Path(tmpdir) / Configuration.FILE_NAME}',
            warn=True,
        ):
            if fix_format:
                print(f'{color_message("Info", Color.BLUE)}: Formatted module configuration file')
                config.to_file()
            else:
                raise Exit(
                    f'{color_message("Error", Color.RED)}: Configuration file is not formatted correctly, use `dda inv modules.validate --fix-format` to fix it'
                )

    with open(base_dir / Configuration.FILE_NAME) as f:
        config_attributes = yaml.safe_load(f)['modules']

    config = Configuration.from_file(base_dir)
    errors = []
    for module in config.modules.values():
        try:
            validate_module(module, config_attributes[module.path], base_dir, default_attributes)
        except AssertionError as e:
            errors.append((module.path, e))

    # Backward check for go.mod (ensure there is a module for each go.mod)
    for go_mod in glob(str(base_dir / '**/go.mod'), recursive=True):
        # Ignore bazel generated symlinks
        if go_mod.startswith(str(base_dir / 'bazel')):
            continue

        path = Path(go_mod).parent.relative_to(base_dir).as_posix()
        assert path in config.modules or path in config.ignored_modules, f"Configuration is missing a module for {path}"

    if errors:
        print(f'{color_message("ERROR", Color.RED)}: Some modules have invalid configurations:')
        for path, error in sorted(errors):
            print(f'- {color_message(path, Color.BOLD)}: {error}')

        raise Exit(f'{color_message("ERROR", Color.RED)}: Found errors in module configurations, see details above')


@task
def validate_used_by_otel(ctx: Context):
    """
    Verify whether indirect local dependencies of modules labeled "used_by_otel" are also marked with the "used_by_otel" tag.
    """
    otel_mods = [path for path, module in get_default_modules().items() if module.used_by_otel]
    missing_used_by_otel_label: dict[str, list[str]] = defaultdict(list)

    # for every module labeled as "used_by_otel"
    default_modules = get_default_modules()
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
            if rel_path not in default_modules or not default_modules[rel_path].used_by_otel:
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


@task
def show(_, path: str, remove_defaults: bool = False, base_dir: str = '.'):
    """Show the module information for the given path.

    Args:
        remove_defaults: If True, will remove default values from the output.
        base_dir: Where to load modules from.
    """

    config = Configuration.from_file(Path(base_dir))
    if path in config.ignored_modules:
        print(f'Module {path} is ignored')
        return

    module = config.modules.get(path)

    assert module, f'Module {path} not found'

    yaml.dump(
        {path: module.to_dict(remove_defaults=remove_defaults, remove_path=True)}, sys.stdout, Dumper=ConfigDumper
    )


@task
def show_all(_, base_dir: str = '.', ignored=False):
    """Show the list of modules.

    Args:
        base_dir: Where to load modules from.
        ignored: If True, will list ignored modules.
    """

    config = Configuration.from_file(Path(base_dir))

    if ignored:
        names = config.ignored_modules
    else:
        names = list(config.modules.keys())

    print('\n'.join(sorted(names)))
    print(len(names), 'modules')


def remove_replace_rules(data: str) -> str:
    # remove all replace block
    data = re.sub("\tgithub.com/DataDog/datadog-agent/.+ => .+", '', data)
    data = re.sub("replace github.com/DataDog/datadog-agent/[^ ]+ => .+", '', data)
    data = re.sub(r"replace \(\s+\)", '', data)
    data = re.sub(r"// This section was automatically added by 'dda inv modules\..+", '', data)
    return data


def update_go_mod(gomod_list, root):
    file = "go.mod"
    repo_name = "github.com/DataDog/datadog-agent/"
    replace_comment = (
        "// This section was automatically added by 'dda inv modules.add-all-replace' command, do not edit manually\n\n"
    )

    gomod_file = os.path.join(root, file)
    print("Updating:", gomod_file)
    with open(gomod_file) as f:
        gomod = f.read()

    prefix = re.sub(r"[^/\.]+", "..", root)
    if prefix.endswith("/"):
        prefix = prefix[:-1]

    # remove all replace block
    gomod = remove_replace_rules(gomod)

    # inject all replace rules at the bottom
    gomod += "\n" + replace_comment
    gomod += "replace (\n"

    for mod in gomod_list:
        if root.endswith(mod):
            # don't add a replace for the current module
            continue
        gomod += f"\t{repo_name}{mod} => {prefix}/{mod}\n"

    gomod += ")\n"

    # Last cleanup: remove concurrent line break
    gomod = re.sub("\n{3,}", "\n\n", gomod)

    with open(os.path.join(root, file), "w") as f:
        f.write(gomod)
        f.truncate()


@task
def add_all_replace(ctx: Context):
    """
    This command will add all the replace rules to all go.mod even if not used. This ensures that go mod tidy will work
    and no replace rule is missing.

    It's meant to be used as the following:
    - running `dda inv modules.add-all-replace` to add all possible replace rules to all go.mod
    - `dda inv tidy` to update all the go.mod

    This solves the problem of `go mod tidy` failing if some replace rules are missing but needing `go mod tidy` to run
    successfully to know which replace rules are needed. This is a major pain point when creating/moving go.mod.

    While this is a brute force approach it's the only way to ensure that all replace are in place while circumventing
    limitations from go toolings:
    - 'go list' only list the dependencies from go mod. This means that if a local version of module as a new dependency
      compare to the latest release version 'go list' will not show it. This means that our previous tooling would not
      detect a missing replace until the local version is released.
    - 'go mod tidy' requires all the replace rules to succeed. But in order of knowing which replace rules are needed we
      need 'go mod tidy' to run successfully.

    After months of pain and manually editing our 150+ go.mod in this repo we have come to this.
    """

    # First we find all go.mod in comp and pkg
    gomods = [
        mods for mods in get_default_modules().values() if mods.path.split(os.sep)[0] not in ["tools", "internal"]
    ]
    mod_to_replace = sorted([mod.path for mod in gomods])
    mod_to_replace.remove(".")

    # Second we iterate over all go.mod and update them
    for mod in gomods:
        if mod.should_replace_internal_modules:
            update_go_mod(mod_to_replace, mod.path)
