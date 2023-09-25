"""
Invoke entrypoint, import here all the tasks we want to make available
"""
import os
import pathlib
from collections import namedtuple
from string import Template

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.copyright import COPYRIGHT_HEADER

Component = namedtuple('Component', ['path', 'doc', 'team'])
Bundle = namedtuple('Component', ['path', 'doc', 'team', 'components'])


def find_team(content):
    for l in content:
        if l.startswith('// team: '):
            return l.split(':', 2)[1].strip()


def find_doc(content):
    comment_block = []
    for l in content:
        if l.startswith('//'):
            comment_block.append(l[3:])
        elif l.startswith('package '):
            try:
                i = comment_block.index('')
                comment_block = comment_block[:i]
            except ValueError:
                pass
            return ''.join(comment_block).strip() + '\n'
        else:
            comment_block = []


def has_type_component(content):
    return any(l.startswith('type Component interface') for l in content)


def get_components_and_bundles(ctx):
    ok = True
    components = []
    bundles = []
    res = ctx.run('git ls-files comp/', hide=True)
    for file in res.stdout.splitlines():
        if file.endswith("/component.go"):
            content = list(open(file, "r"))
            if not has_type_component(content):
                print(f"** {file} does not define a Component interface; skipping")
                ok = False
                pass

            path = file[: -len('/component.go')]
            team = find_team(content)
            doc = find_doc(content)

            if team is None:
                print(f"** {file} does not name a responsible team")
                ok = False

            components.append(Component(path, doc, team))

        elif file.endswith("/bundle.go"):
            content = list(open(file, "r"))
            if has_type_component(content):
                print(f"** {file} defines a Component interface (bundles should not do so)")
                ok = False
                pass

            path = file[: -len('/bundle.go')]
            team = find_team(content)
            doc = find_doc(content)

            if team is None:
                print(f"** {file} does not name a responsible team")
                ok = False

            bundles.append(Bundle(path, doc, team, []))

    # assign components to bundles
    bundles = [Bundle(b.path, b.doc, b.team, [c for c in components if c.path.startswith(b.path)]) for b in bundles]

    # look for un-bundled components
    for c in components:
        if not any(c in b.components for b in bundles):
            print(f"** component {c.path} is not in any bundle")
            ok = False

    return sorted(bundles), ok


def make_components_md(bundles):
    pkg_root = 'github.com/DataDog/dd-agent-comp-experiments/'
    yield '# Agent Components'
    yield '<!-- NOTE: this file is auto-generated; do not edit -->'
    yield ''
    yield 'This file lists all components defined in this repository, with their package summary.'
    yield 'Click the links for more documentation.'
    yield ''
    for b in bundles:
        yield f'## [{b.path}](https://pkg.go.dev/{pkg_root}{b.path}) (Component Bundle)'
        yield ''
        yield f'*Datadog Team*: {b.team}'
        yield ''
        yield b.doc
        for c in b.components:
            yield f'### [{c.path}](https://pkg.go.dev/{pkg_root}{c.path})'
            yield ''
            if c.team != b.team:
                yield f'*Datadog Team*: {c.team}'
                yield ''
            yield c.doc


def make_codeowners(codeowners_lines, bundles):
    codeowners_lines = codeowners_lines.__iter__()

    # pass through the codeowners lines up to and including "# BEGIN COMPONENTS"
    for line in codeowners_lines:
        yield line
        if line == "# BEGIN COMPONENTS":
            break

    # codeowners is parsed in a last-match-wins fashion, so put more-specific values (components) after
    # less-specific (bundles).  We include only components with a team different from their bundle, to
    # keep the file short.
    yield '/comp @DataDog/agent-shared-components'
    different_components = []
    for b in bundles:
        if b.team:
            yield f'/{b.path} @DataDog/{b.team}'
        for c in b.components:
            if c.team != b.team:
                different_components.append(c)
    for c in different_components:
        if c.team:
            yield f'/{c.path} @DataDog/{c.team}'

    # drop lines from the existing codeowners until "# END COMPONENTS"
    for line in codeowners_lines:
        if line == "# END COMPONENTS":
            yield line
            break

    # pass through the rest of the file
    for line in codeowners_lines:
        yield line

    # ensure there's a trailing newline in the file
    yield ""


@task
def lint_components(ctx, fix=False):
    """
    Verify (or with --fix, ensure) component-related things are correct.
    """
    bundles, ok = get_components_and_bundles(ctx)
    fixable = False

    # Check comp/README.md
    filename = "comp/README.md"
    components_md = '\n'.join(make_components_md(bundles))
    if fix:
        with open(filename, "w") as f:
            f.write(components_md)
    else:
        with open(filename, "r") as f:
            current = f.read()
            if current != components_md:
                print(f"** {filename} differs")
                ok = False
                fixable = True

    # Check .github/CODEOWNERS
    filename = ".github/CODEOWNERS"
    with open(filename, "r") as f:
        current = f.read()
    codeowners = '\n'.join(make_codeowners(current.splitlines(), bundles))
    if fix:
        with open(".github/CODEOWNERS", "w") as f:
            f.write(codeowners)
    elif current != codeowners:
        print(f"** {filename} differs")
        ok = False
        fixable = True

    if not ok:
        if fixable:
            print("Run `inv lint-components --fix` to fix errors")
        raise Exit(code=1)


@task
def new_bundle(_, bundle_path, overwrite=False, team="/* TODO: add team name */"):
    """
    Create a new bundle package with bundle.go and bundle_test.go files.

    Notes:
        - This task must be called from the datadog-agent repository root folder.
        - 'bundle-path' is not modified by the task. You should explicitly set this to 'comp/...' if you want to create it in the right folder.
        - You can use the --team flag to set the team name for the new bundle.

    Examples:
        inv components.new-bundle comp/foo/bar             # Create the 'bar' bundle in the 'comp/foo' folder
        inv components.new-bundle comp/foo/bar --overwrite # Create the 'bar' bundle in the 'comp/foo' folder and overwrite 'comp/foo/bar/bundle{_test}.go' even if they already exist.
        inv components.new-bundle /tmp/baz                 # Create the 'baz' bundle in the '/tmp/' folder. './comp' prefix is not enforced by the task.
    """
    template_var_mapping = {"BUNDLE_NAME": os.path.basename(bundle_path), "TEAM_NAME": team}
    create_components_framework_files(bundle_path, ["bundle.go", "bundle_test.go"], template_var_mapping, overwrite)


@task
def new_component(_, comp_path, overwrite=False, team="/* TODO: add team name */"):
    """
    Create a new component package with the component.go file.

    Notes:
        - This task must be called from the datadog-agent repository root folder.
        - 'comp-path' is not modified by the task. You should explicitly set this to 'comp/...' if you want to create it in the right folder.
        - You can use the --team flag to set the team name for the new component/

    Examples:
        inv components.new-component comp/foo/bar             # Create the 'bar' component in the 'comp/foo' folder
        inv components.new-component comp/foo/bar --overwrite # Create the 'bar' component in the 'comp/foo' folder and overwrite 'comp/foo/bar/component.go' even if it already exists
        inv components.new-component /tmp/baz                 # Create the 'baz' component in the '/tmp/' folder. './comp' prefix is not enforced by the task.
    """
    template_var_mapping = {"COMPONENT_NAME": os.path.basename(comp_path), "TEAM_NAME": team}
    create_components_framework_files(comp_path, ["component.go"], template_var_mapping, overwrite)


def create_components_framework_files(comp_path, new_files, template_var_mapping, overwrite):
    """
    Create the folder and files common to all components and bundles.

    First this function create the 'comp_path' folder. Then, for each file path in the 'new_files' list, it creates files
    with a specific content. The content of each file is given by a predefined template located in the 'tasks/components_templates' folder.

    These templates are Golang files with variables that can be substituted. These variables names and values are defined in the
    'template_var_mapping' dictionary.

    Lastly, 'overwrite' is a boolean which allows the tasks to erase files in 'new_files' if they already exists
    """
    # Only for logging purpose
    comp_type = "component" if "COMPONENT_NAME" in template_var_mapping else "bundle"

    if not comp_path.startswith("comp/") and not comp_path.startswith("./comp/"):
        print(
            f"Warn: Input path '{comp_path}' does not start with 'comp/'. Your {comp_type} might not be created in the right place."
        )

    component_name = os.path.basename(comp_path)
    if os.path.isdir(comp_path) and not overwrite:
        raise Exit(
            f"Error: Cannot create {component_name} {comp_type}: '{comp_path}' package already exists. Use `--overwrite` if you want to overwrite files in this package.",
            code=1,
        )

    # Create the root folder. We temporary set the umask to 0 to prevent 'os.makedirs' from giving wrong permissions to subfolders
    try:
        print(f"Creating {comp_path} folder")
        # os.makedirs creates all parents directory with 0o777 permissions, 'mode' is only used for the leaf folder.
        # We set the umask to create folder with 0o755 permissions instead of 0o777
        original_umask = os.umask(0o022)
        os.makedirs(comp_path, mode=0o755, exist_ok=True)
    except Exception as err:
        print(err)
    finally:
        os.umask(original_umask)

    # Create the components framework common files from predefined templates
    for filename in new_files:
        write_template(f"{comp_path}/{filename}", template_var_mapping, overwrite)


def write_template(new_file_path, var_mapping, overwrite=False):
    """
    Get the content of a templated file, substitute its variables and then writes the result into 'new_file_path' file.
    """
    # Get the content of the template and resolve it
    template_path = get_template_path(new_file_path)
    raw_template_value = read_file_content(template_path)

    var_mapping["COPYRIGHT_HEADER"] = COPYRIGHT_HEADER
    resolved_template = Template(raw_template_value).substitute(var_mapping)

    # Fails if file exists and 'overwrite' is False
    mode = "w" if overwrite else "x"
    with open(new_file_path, mode) as file:
        file.write(resolved_template)
        print(f"Writing to {new_file_path}")


def get_template_path(file_path):
    """
    Return a path to the template associated with 'file_path'.

    Templates are static files containing variables whose value can be substituted at runtime.
    These templates are used to generate Golang files that are always the same except for some parts such as package name.

    These templates are located in the `tasks/components_templates` folder.

    For instance, if called with `component.go`, the functions returns 'tasks/components_templates/component.go.tmpl'
    """

    template_folder_path = "tasks/components_templates/"
    template_name = os.path.basename(file_path) + ".tmpl"
    return os.path.join(template_folder_path, template_name)


def read_file_content(template_path):
    """
    Read all lines in files and return them as a single string.
    """
    with open(template_path, "r") as file:
        return file.read()


@task
def lint_fxutil_oneshot_test(_):
    """
    Verify each fxutil.OneShot has an unit test
    """
    folders = ["./cmd", "./pkg/cli", "./comp"]
    errors = []
    for folder in folders:
        folder_path = pathlib.Path(folder)
        for file in folder_path.rglob("*.go"):
            if str(file).endswith("_test.go") or str(file).endswith("main.go"):
                continue

            # The code in this file cannot be easily tested
            if "cmd/system-probe/subcommands/run/command.go" in str(file):
                continue

            one_shot_count = file.read_text().count("fxutil.OneShot(")
            if one_shot_count > 0:
                test_path = file.parent.joinpath(f"{file.stem}_test.go")
                if not test_path.exists():
                    errors.append(f"The file {file} contains fxutil.OneShot but the file {test_path} doesn't exist.")
                else:
                    test_one_shot_count = test_path.read_text().count("fxutil.TestOneShotSubcommand(")
                    if one_shot_count > test_one_shot_count:
                        errors.append(
                            f"The file {file} contains {one_shot_count} call(s) to `fxutil.OneShot` but {test_path} contains only {test_one_shot_count} call(s) to `fxutil.TestOneShotSubcommand`"
                        )
    if len(errors) > 0:
        msg = '\n'.join(errors)
        raise Exit(f"Missings tests: {msg}")
