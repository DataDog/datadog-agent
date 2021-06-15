"""
Invoke entrypoint, import here all the tasks we want to make available
"""
import os

from invoke import Collection

from tasks.utils import generate_config

from . import (
    agent,
    android,
    bench,
    cluster_agent,
    cluster_agent_cloudfoundry,
    customaction,
    docker,
    dogstatsd,
    github,
    installcmd,
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
    uninstallcmd,
)
from .build_tags import audit_tag_impact
from .go import (
    check_mod_tidy,
    cyclo,
    deps,
    deps_vendored,
    fmt,
    generate,
    generate_licenses,
    generate_protobuf,
    golangci_lint,
    lint,
    lint_licenses,
    reset,
    tidy_all,
    vet,
)
from .test import (
    check_gitlab_broken_dependencies,
    e2e_tests,
    install_shellcheck,
    install_tools,
    integration_tests,
    lint_filenames,
    lint_milestone,
    lint_python,
    lint_releasenote,
    lint_teamassignment,
    make_kitchen_gitlab_yml,
    make_simple_gitlab_yml,
    test,
)

# the root namespace
ns = Collection()

# add single tasks to the root
ns.add_task(fmt)
ns.add_task(lint)
ns.add_task(vet)
ns.add_task(cyclo)
ns.add_task(golangci_lint)
ns.add_task(test)
ns.add_task(integration_tests)
ns.add_task(deps)
ns.add_task(deps_vendored)
ns.add_task(lint_licenses)
ns.add_task(generate_licenses)
ns.add_task(generate_protobuf)
ns.add_task(reset)
ns.add_task(lint_teamassignment)
ns.add_task(lint_releasenote)
ns.add_task(lint_milestone)
ns.add_task(lint_filenames)
ns.add_task(lint_python)
ns.add_task(audit_tag_impact)
ns.add_task(e2e_tests)
ns.add_task(make_kitchen_gitlab_yml)
ns.add_task(make_simple_gitlab_yml)
ns.add_task(check_gitlab_broken_dependencies)
ns.add_task(generate)
ns.add_task(install_shellcheck)
ns.add_task(install_tools)
ns.add_task(check_mod_tidy)
ns.add_task(tidy_all)
ns.add_task(generate_config)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(android)
ns.add_collection(cluster_agent)
ns.add_collection(cluster_agent_cloudfoundry)
ns.add_collection(customaction)
ns.add_collection(installcmd)
ns.add_collection(bench)
ns.add_collection(trace_agent)
ns.add_collection(docker)
ns.add_collection(dogstatsd)
ns.add_collection(github)
ns.add_collection(pipeline)
ns.add_collection(pylauncher)
ns.add_collection(selinux)
ns.add_collection(systray)
ns.add_collection(release)
ns.add_collection(rtloader)
ns.add_collection(system_probe)
ns.add_collection(process_agent)
ns.add_collection(uninstallcmd)
ns.add_collection(security_agent)

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
