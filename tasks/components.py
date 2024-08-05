"""
Invoke entrypoint, import here all the tasks we want to make available
"""

import os
import pathlib
from collections import namedtuple
from collections.abc import Iterable
from string import Template

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.types.copyright import COPYRIGHT_HEADER

# Component represents a directory defining a component
#  version=1 is the classic style using:
#    comp/<name>/component.go
#    comp/<name>/<name>impl/*
#  version=2 is the new style using:
#    comp/<name>/def/component.go
#    comp/<name>/fx/fx.go
#    comp/<name>/impl/*
Component = namedtuple('Component', ['name', 'def_file', 'path', 'doc', 'team', 'version'])
# Bundle represents a bundle of components, defined using:
#    comp/<group>/bundle.go
Bundle = namedtuple('Bundle', ['path', 'doc', 'team', 'content', 'components'])


def find_team(content: Iterable[str]) -> str | None:
    for line in content:
        if line.startswith('// team: '):
            return line.split(':', 2)[1].strip()

    return None


def find_doc(content) -> str:
    comment_block = []
    first_paragraph_only = True

    for line in content:
        if line.startswith('//'):
            text = line[3:].strip()
            if not text:
                first_paragraph_only = False
            if first_paragraph_only:
                comment_block.append(text + '\n')
        elif line.startswith('package '):
            break
        else:
            comment_block = []
            first_paragraph_only = True
    return ''.join(comment_block).strip() + '\n'


def has_type_component(content) -> bool:
    return any(line.startswith('type Component interface') for line in content)


# // TODO: (components)
# The migration of these components is in progresss.
# Please do not add a new component to this list.
components_to_migrate = [
    "comp/aggregator/demultiplexer/component.go",
    "comp/core/config/component.go",
    "comp/core/flare/component.go",
    "comp/dogstatsd/server/component.go",
    "comp/forwarder/defaultforwarder/component.go",
    "comp/metadata/inventoryagent/component.go",
    "comp/netflow/config/component.go",
    "comp/netflow/server/component.go",
    "comp/remote-config/rcclient/component.go",
    "comp/trace/config/component.go",
    "comp/process/apiserver/component.go",
]


# List of components that use the classic style, where `comp/<component>/<component>impl` exists
# New components should use the new style of `def`, `impl`, `fx` folders
components_classic_style = [
    'comp/agent/autoexit/autoexitimpl',
    'comp/agent/cloudfoundrycontainer/cloudfoundrycontainerimpl',
    'comp/agent/expvarserver/expvarserverimpl',
    'comp/agent/jmxlogger/jmxloggerimpl',
    'comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl',
    'comp/api/api/apiimpl',
    'comp/api/api/def',
    'comp/api/authtoken/fetchonlyimpl',
    'comp/api/authtoken/createandfetchimpl',
    'comp/checks/agentcrashdetect/agentcrashdetectimpl',
    'comp/checks/windowseventlog/windowseventlogimpl',
    "comp/checks/winregistry/impl",
    'comp/collector/collector/collectorimpl',
    'comp/core/autodiscovery/autodiscoveryimpl',
    'comp/core/configsync/configsyncimpl',
    'comp/core/gui/guiimpl',
    'comp/core/hostname/hostnameimpl',
    'comp/core/log/logimpl',
    'comp/core/log/tracelogimpl',
    'comp/core/pid/pidimpl',
    'comp/core/secrets/secretsimpl',
    'comp/core/settings/settingsimpl',
    'comp/core/status/statusimpl',
    'comp/core/sysprobeconfig/sysprobeconfigimpl',
    'comp/core/telemetry/telemetryimpl',
    'comp/core/telemetry/noopsimpl',
    'comp/dogstatsd/pidmap/pidmapimpl',
    'comp/dogstatsd/serverDebug/serverdebugimpl',
    'comp/dogstatsd/status/statusimpl',
    'comp/etw/impl',
    'comp/forwarder/eventplatform/eventplatformimpl',
    'comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl',
    'comp/forwarder/orchestrator/orchestratorimpl',
    'comp/languagedetection/client/clientimpl',
    'comp/logs/adscheduler/adschedulerimpl',
    'comp/logs/agent/agentimpl',
    'comp/metadata/host/hostimpl',
    'comp/metadata/inventorychecks/inventorychecksimpl',
    'comp/metadata/inventoryhost/inventoryhostimpl',
    'comp/metadata/inventoryotel/inventoryotelimpl',
    'comp/metadata/packagesigning/packagesigningimpl',
    'comp/metadata/resources/resourcesimpl',
    'comp/metadata/runner/runnerimpl',
    'comp/ndmtmp/forwarder/forwarderimpl',
    'comp/networkpath/npcollector/npcollectorimpl',
    'comp/otelcol/logsagentpipeline/logsagentpipelineimpl',
    'comp/process/agent/agentimpl',
    'comp/process/connectionscheck/connectionscheckimpl',
    'comp/process/containercheck/containercheckimpl',
    'comp/process/expvars/expvarsimpl',
    'comp/process/forwarders/forwardersimpl',
    'comp/process/hostinfo/hostinfoimpl',
    'comp/process/processcheck/processcheckimpl',
    'comp/process/processdiscoverycheck/processdiscoverycheckimpl',
    'comp/process/processeventscheck/processeventscheckimpl',
    'comp/process/profiler/profilerimpl',
    'comp/process/rtcontainercheck/rtcontainercheckimpl',
    'comp/process/runner/runnerimpl',
    'comp/process/status/statusimpl',
    'comp/process/submitter/submitterimpl',
    'comp/remote-config/rcservice/rcserviceimpl',
    'comp/remote-config/rcservicemrf/rcservicemrfimpl',
    'comp/remote-config/rcstatus/rcstatusimpl',
    'comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl',
    'comp/serializer/compression/compressionimpl',
    'comp/snmptraps/config/configimpl',
    'comp/snmptraps/formatter/formatterimpl',
    'comp/snmptraps/forwarder/forwarderimpl',
    'comp/snmptraps/listener/listenerimpl',
    'comp/snmptraps/oidresolver/oidresolverimpl',
    'comp/snmptraps/server/serverimpl',
    'comp/snmptraps/status/statusimpl',
    'comp/systray/systray/systrayimpl',
    'comp/trace/etwtracer/etwtracerimpl',
    'comp/trace/status/statusimpl',
    'comp/updater/localapi/localapiimpl',
    'comp/updater/localapiclient/localapiclientimpl',
    'comp/updater/telemetry/telemetryimpl',
    'comp/updater/updater/updaterimpl',
]


# // TODO: (components)
# The migration of these components is in progresss.
# Please do not add a new component to this list.
components_missing_implementation_folder = [
    "comp/dogstatsd/statsd",
    "comp/core/tagger",
    "comp/forwarder/orchestrator/orchestratorinterface",
    "comp/core/hostname/hostnameinterface",
]

ignore_fx_import = [
    "comp/core/workloadmeta",
    "comp/rdnsquerier",
    "comp/trace/agent",
]

ignore_provide_component_constructor_missing = [
    "comp/core/workloadmeta",
    "comp/trace/agent",
]

mock_definitions = [
    "type Mock interface",
    "func Module() fxutil.Module",
    "func MockModule() fxutil.Module",
]


def check_component_contents_and_file_hiearchy(comp):
    """
    Check validity of component, returning first error, if any found
    """
    def_content = read_file_content(comp.def_file).split('\n')
    root_path = pathlib.Path(comp.path)

    # Definition file `def/component.go` must define a component interface
    if not any(
        line.startswith('type Component interface') or line.startswith('type Component = ') for line in def_content
    ):
        return f"** {comp.def_file} does not define a Component interface"

    # Skip components that need to migrate
    if str(comp.def_file) in components_to_migrate:
        return

    # Special case for api
    if comp.def_file == 'comp/api/api/def/component.go':
        return

    # Definition file `component.go` (v1) or `def/component.go` (v2) must use `package <compname>`
    pkgname = parse_package_name(comp.def_file)
    if pkgname != comp.name:
        return f"** {comp.def_file} has wrong package name '{pkgname}', must be '{comp.name}'"

    # Definition file `component.go` (v1) or `def/component.go` (v2) must not contain a mock definition
    for mock_definition in mock_definitions:
        if any(line.startswith(mock_definition) for line in def_content):
            return f"** {comp.def_file} defines '{mock_definition}' which should be in separate implementation. See docs/components/defining-components.md"

    # Allowlist of components that do not use an implementation folder
    if comp.path in components_missing_implementation_folder:
        return

    # Implementation folder or folders must exist
    impl_folders = locate_implementation_folders(comp)
    if len(impl_folders) == 0:
        return f"** {comp.name} is missing the implementation folder in {comp.path}. See docs/components/defining-components.md"

    if comp.version == 2:
        # Implementation source files should use correct package name, and shouldn't import fx (except tests)
        for src_file in locate_nontest_source_files(impl_folders):
            pkgname = parse_package_name(src_file)
            expectname = comp.name + 'impl'
            if pkgname != expectname:
                return f"** {src_file} has wrong package name '{pkgname}', must be '{expectname}'"
            if comp.path in ignore_fx_import:
                continue
            src_content = read_file_content(src_file)
            if 'go.uber.org/fx' in src_content:
                return f"** {src_file} should not import 'go.uber.org/fx' because it a component implementation"
            if 'fxutil' in src_content:
                return f"** {src_file} should not import 'fxutil' because it a component implementation"
        # FX files should use correct filename and package name, and call ProvideComponentConstructor
        for src_file in locate_fx_source_files(root_path):
            if src_file.name != 'fx.go':
                return f"** {src_file} should be named 'fx.go'"
            pkgname = parse_package_name(src_file)
            expectname = comp.name + 'fx'
            if pkgname != 'fx' and pkgname != expectname:
                return f"** {src_file} has wrong package name '{pkgname}', must be 'fx' or '{expectname}'"
            if comp.path in ignore_provide_component_constructor_missing:
                continue
            src_content = read_file_content(src_file)
            if 'ProvideComponentConstructor' not in src_content:
                return f"** {src_file} should call ProvideComponentConstructor to convert regular constructor into fx-aware"

    return  # no error


def parse_package_name(filename):
    """
    Return the package name from the given filename
    """
    lines = read_file_content(filename).split('\n')
    pkgline = [line for line in lines if line.startswith('package ')][0]
    results = pkgline.split(' ')
    if len(results) < 2:
        return None
    return results[1]


def locate_implementation_folders(comp):
    """
    Return all implementation folders from the component
    """
    root_path = pathlib.Path(comp.path)
    folders = []

    for entry in root_path.iterdir():
        if entry.is_file():
            continue

        if str(entry) in components_missing_implementation_folder:
            return 'skip'

        if comp.version == 2:
            # Check for component implementation using the new-style folder structure: comp/<component>/impl[-suffix]
            if entry.match('impl-*') or entry.match('impl'):
                folders.append(entry)

        if comp.version == 1:
            # Check for component implementation using the classic style: comp/<component>/<component>impl
            if str(entry) in components_classic_style:
                folders.append(entry)

    return folders


def locate_nontest_source_files(folder_list):
    """
    Return all non-test source files from given list of folders
    """
    results = []
    for folder in folder_list:
        for entry in folder.iterdir():
            if not entry.is_file():
                continue
            filename = str(entry)
            if filename.endswith('.go') and not filename.endswith('_test.go'):
                results.append(entry)
    return results


def locate_fx_source_files(root_path):
    """
    Return all source files from the fx subfolder in the component's path
    """
    results = []
    for entry in root_path.iterdir():
        if entry.is_file():
            continue
        if entry.name.startswith('fx'):
            for subentry in entry.iterdir():
                results.append(subentry)
    return results


def validate_components(components, errs=None):
    if errs is None:
        errs = []
    for c in components:
        e = check_component_contents_and_file_hiearchy(c)
        if e is not None and len(e) > 0:
            errs.append(e)
        if c.team is None:
            errs.append(f"** {c.path} does not specify a team owner")
    return errs


def validate_bundles(bundles, errs=None):
    if errs is None:
        errs = []
    errs = []
    for bundle in bundles:
        # bundle should not declare an interface
        if has_type_component(bundle.content):
            errs.append(f"** {bundle.path} defines a Component interface (bundles should not do so)")
        # bundle should declare team owner
        if bundle.team is None:
            errs.append(f"** {bundle.path} does not specify a team owner")
    return errs


def get_components_and_bundles():
    """
    Traverse comp/ directory and return all components, plus all bundles
    """
    components = []
    bundles = []
    for component_file in pathlib.Path('comp').glob('**'):
        if not component_file.is_dir():
            continue

        component_directory = pathlib.Path(component_file)
        for direntry in component_directory.iterdir():
            if direntry.is_file() and direntry.name == "bundle.go":
                # Found bundle definition
                content = read_file_content(direntry).split('\n')
                path = str(direntry)[: -len('/bundle.go')]
                team = find_team(content)
                doc = find_doc(content)
                bundles.append(Bundle(path, doc, team, content, []))
                continue

            comp = locate_component_def(direntry)
            if comp is not None:
                # Found component definition
                components.append(comp)

    # assign components to bundles
    sorted_bundles = []
    for b in bundles:
        bundle_components = []
        for c in components:
            if c.path.startswith(b.path):
                bundle_components.append(c)
        sorted_bundles.append(Bundle(b.path, b.doc, b.team, b.content, sorted(bundle_components)))

    return sorted(components, key=lambda c: c.path), sorted(sorted_bundles)


def locate_component_def(dir):
    """
    Locate the component, if this directory contains a component
    """
    component_name = dir.name.replace('-', '').lower()

    # v2 component: this folder is a component root if it contains 'def/component.go'
    def_file = dir / 'def/component.go'
    if def_file.is_file():
        # comp/api/api/def/component.go is a special case, it's not a component using version 2
        # PLEASE DO NOT ADD MORE EXCEPTIONS
        if str(def_file) == "comp/api/api/def/component.go":
            return construct_component(component_name, def_file, dir, 1)
        else:
            return construct_component(component_name, def_file, dir, 2)

    # v1 component: this folder is a component root if it contains '/component.go' but the path is not '/def/component.go'
    # in particular, the directory named 'def' should not be treated as a component root
    def_file = dir / 'component.go'
    if def_file.is_file() and '/def/component.go' not in str(def_file):
        return construct_component(component_name, def_file, dir, 1)


def construct_component(compname, def_file, path, version):
    def_content = read_file_content(str(def_file)).split('\n')
    team = find_team(def_content)
    doc = find_doc(def_content)
    return Component(compname, str(def_file), str(path), doc, team, version)


def make_components_md(bundles, components_without_bundle):
    pkg_root = 'github.com/DataDog/datadog-agent/'
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
    for c in components_without_bundle:
        yield f'### [{c.path}](https://pkg.go.dev/{pkg_root}{c.path})'
        yield ''
        if c.team != b.team:
            yield f'*Datadog Team*: {c.team}'
            yield ''
        yield c.doc


def build_codeowner_entry(path, team):
    teams = [f'@DataDog/{team}' for team in team.split(' ')]
    return f'/{path} ' + ' '.join(teams)


def make_codeowners(codeowners_lines, bundles, components_without_bundle):
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
            yield build_codeowner_entry(b.path, b.team)
        for c in b.components:
            if c.team != b.team:
                different_components.append(c)
    for c in different_components:
        if c.team:
            yield build_codeowner_entry(c.path, c.team)
    for c in components_without_bundle:
        yield build_codeowner_entry(c.path, c.team)

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
def lint_components(_, fix=False):
    """
    Verify (or with --fix, ensure) component-related things are correct.
    """
    components, bundles = get_components_and_bundles()
    ok = True
    fixable = False
    errs = []

    errs = validate_components(components, errs)
    if len(errs) > 0:
        for err in errs:
            ok = False
            print(err)

    errs = validate_bundles(bundles, errs)
    if len(errs) > 0:
        for err in errs:
            ok = False
            print(err)

    # Filter just components that are not included in any bundles
    components_without_bundle = without_bundle(components, bundles)

    # Check comp/README.md
    filename = "comp/README.md"
    components_md = '\n'.join(make_components_md(bundles, components_without_bundle))
    if fix:
        with open(filename, "w") as f:
            f.write(components_md)
    else:
        with open(filename) as f:
            current = f.read()
            if current != components_md:
                print(f"** {filename} differs")
                ok = False
                fixable = True

    # Check .github/CODEOWNERS
    filename = ".github/CODEOWNERS"
    with open(filename) as f:
        current = f.read()
    codeowners = '\n'.join(make_codeowners(current.splitlines(), bundles, components_without_bundle))
    if fix:
        with open(".github/CODEOWNERS", "w") as f:
            f.write(codeowners)
    elif current != codeowners:
        print(f"** {filename} differs")
        ok = False
        fixable = True

    if not ok:
        if fixable:
            print("Run `inv components.lint-components --fix` to fix errors")
        raise Exit(code=1)


def without_bundle(comps, bundles):
    ans = []
    for c in comps:
        if not any(c in b.components for b in bundles):
            ans.append(c)
    return sorted(ans, key=lambda c: c.path)


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
    create_components_framework_files(
        bundle_path, [("bundle.go", "bundle.go"), ("bundle_test.go", "bundle_test.go")], template_var_mapping, overwrite
    )


@task
def new_component(_, comp_path, overwrite=False, team="/* TODO: add team name */"):
    """
    Create a new component package with default files.

    Notes:
        - This task must be called from the datadog-agent repository root folder.
        - 'comp-path' is not modified by the task. You should explicitly set this to 'comp/...' if you want to create it in the right folder.
        - You can use the --team flag to set the team name for the new component/

    Examples:
        inv components.new-component comp/foo/bar             # Create the 'bar' component in the 'comp/foo' folder
        inv components.new-component comp/foo/bar --overwrite # Create the 'bar' component in the 'comp/foo' folder and overwrite 'comp/foo/bar/component.go' even if it already exists
        inv components.new-component /tmp/baz                 # Create the 'baz' component in the '/tmp/' folder. './comp' prefix is not enforced by the task.
    """
    component_name = os.path.basename(comp_path)
    template_var_mapping = {
        "COMPONENT_PATH": comp_path,
        "COMPONENT_NAME": component_name,
        "CAPITALIZED_COMPONENT_NAME": component_name.capitalize(),
        "TEAM_NAME": team,
    }
    create_components_framework_files(
        comp_path,
        [
            ("def/component.go", "def/component.go"),
            ("fx/fx.go", "fx/fx.go"),
            (os.path.join("impl", f"{component_name}.go"), "impl/component.go"),
            ("mock/mock.go", "mock/mock.go"),
        ],
        template_var_mapping,
        overwrite,
    )


def create_components_framework_files(comp_path, new_paths, template_var_mapping, overwrite):
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
    for path, template_path in new_paths:
        folder = os.path.dirname(path)
        os.makedirs(os.path.join(comp_path, folder), exist_ok=True)
        write_template(comp_path, template_path, path, template_var_mapping, overwrite)


def write_template(comp_path, template_name, new_file_path, var_mapping, overwrite=False):
    """
    Get the content of a templated file, substitute its variables and then writes the result into 'new_file_path' file.
    """
    template_path = get_template_path(template_name)
    # Get the content of the template and resolve it
    raw_template_value = read_file_content(template_path)

    var_mapping["COPYRIGHT_HEADER"] = COPYRIGHT_HEADER
    resolved_template = Template(raw_template_value).substitute(var_mapping)

    # Fails if file exists and 'overwrite' is False
    mode = "w" if overwrite else "x"
    full_path = os.path.join(comp_path, new_file_path)
    with open(full_path, mode) as file:
        file.write(resolved_template)
        print(f"Writing to {full_path}")


def get_template_path(relative_path):
    return os.path.join("tasks", "components_templates", relative_path + ".tmpl")


def read_file_content(template_path):
    """
    Read all lines in files and return them as a single string.
    """
    with open(template_path) as file:
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
            # Don't lint test files
            if str(file).endswith("_test.go"):
                continue

            one_shot_count = file.read_text().count("fxutil.OneShot(")
            run_count = file.read_text().count("fxutil.Run(")

            expect_reason = 'fxutil.OneShot'
            if one_shot_count == 0 and run_count > 0:
                expect_reason = 'fxutil.Run'

            if one_shot_count > 0 or run_count > 0:
                test_path = file.parent.joinpath(f"{file.stem}_test.go")
                if not test_path.exists():
                    errors.append(f"The file {file} contains {expect_reason} but the file {test_path} doesn't exist.")
                else:
                    content = test_path.read_text()
                    test_sub_cmd_count = content.count("fxutil.TestOneShotSubcommand(")
                    test_one_shot_count = content.count("fxutil.TestOneShot(")
                    test_run_count = content.count("fxutil.TestRun(")
                    if one_shot_count > test_sub_cmd_count + test_one_shot_count:
                        errors.append(
                            f"The file {file} contains {one_shot_count} call(s) to `fxutil.OneShot`"
                            + f" but {test_path} contains only {test_sub_cmd_count} call(s) to `fxutil.TestOneShotSubcommand`"
                            + f" and {test_one_shot_count} call(s) to `fxutil.TestOneShot`"
                        )
                    if run_count > test_run_count:
                        errors.append(
                            f"The file {file} contains {run_count} call(s) to `fxutil.Run`"
                            + f" but {test_path} contains only {test_run_count} call(s) to `fxutil.TestRun`"
                        )
    if len(errors) > 0:
        msg = '\n'.join(errors)
        raise Exit(f"Missings tests: {msg}")
