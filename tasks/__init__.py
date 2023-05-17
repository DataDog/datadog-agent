"""
Invoke entrypoint, import here all the tasks we want to make available
"""
import os

from invoke import Collection

from . import (
    agent,
    bench,
    cluster_agent,
    cluster_agent_cloudfoundry,
    components,
    customaction,
    docker,
    dogstatsd,
    epforwarder,
    github,
    msi,
    new_e2e_tests,
    package,
    pipeline,
    process_agent,
    pylauncher,
    release,
    rtloader,
    security_agent,
    selinux,
    system_probe,
    systray,
    trace_agent,
    vscode,
)
from .build_tags import audit_tag_impact, print_default_build_tags
from .components import lint_components
from .fuzz import fuzz
from .go import (
    check_go_version,
    check_mod_tidy,
    deps,
    deps_vendored,
    generate_licenses,
    generate_protobuf,
    go_fix,
    golangci_lint,
    lint_licenses,
    reset,
    tidy_all,
)
from .test import (
    codecov,
    download_tools,
    e2e_tests,
    install_shellcheck,
    install_tools,
    integration_tests,
    invoke_unit_tests,
    junit_macos_repack,
    junit_upload,
    lint_copyrights,
    lint_filenames,
    lint_go,
    lint_milestone,
    lint_python,
    lint_releasenote,
    lint_teamassignment,
    test,
)
from .utils import generate_config

# the root namespace
ns = Collection()

# add single tasks to the root
ns.add_task(golangci_lint)
ns.add_task(test)
ns.add_task(codecov)
ns.add_task(integration_tests)
ns.add_task(deps)
ns.add_task(deps_vendored)
ns.add_task(lint_licenses)
ns.add_task(generate_licenses)
ns.add_task(lint_components)
ns.add_task(generate_protobuf)
ns.add_task(reset)
ns.add_task(lint_copyrights),
ns.add_task(lint_teamassignment)
ns.add_task(lint_releasenote)
ns.add_task(lint_milestone)
ns.add_task(lint_filenames)
ns.add_task(lint_python)
ns.add_task(lint_go)
ns.add_task(audit_tag_impact)
ns.add_task(print_default_build_tags)
ns.add_task(e2e_tests)
ns.add_task(install_shellcheck)
ns.add_task(download_tools)
ns.add_task(install_tools)
ns.add_task(invoke_unit_tests)
ns.add_task(check_mod_tidy)
ns.add_task(tidy_all)
ns.add_task(check_go_version)
ns.add_task(generate_config)
ns.add_task(junit_upload)
ns.add_task(junit_macos_repack)
ns.add_task(fuzz)
ns.add_task(go_fix)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(cluster_agent)
ns.add_collection(cluster_agent_cloudfoundry)
ns.add_collection(components)
ns.add_collection(customaction)
ns.add_collection(bench)
ns.add_collection(trace_agent)
ns.add_collection(docker)
ns.add_collection(dogstatsd)
ns.add_collection(epforwarder)
ns.add_collection(msi)
ns.add_collection(github)
ns.add_collection(package)
ns.add_collection(pipeline)
ns.add_collection(pylauncher)
ns.add_collection(selinux)
ns.add_collection(systray)
ns.add_collection(release)
ns.add_collection(rtloader)
ns.add_collection(system_probe)
ns.add_collection(process_agent)
ns.add_collection(security_agent)
ns.add_collection(vscode)
ns.add_collection(new_e2e_tests)
ns.configure(
    {
        'run': {
            # workaround waiting for a fix being merged on Invoke,
            # see https://github.com/pyinvoke/invoke/pull/407
            'shell': os.environ.get('COMSPEC', os.environ.get('SHELL')),
            # this should stay, set the encoding explicitly so invoke doesn't
            # freak out if a command outputs unicode chars.
            'encoding': 'utf-8',
        }
    }
)
