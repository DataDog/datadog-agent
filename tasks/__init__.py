"""
Invoke entrypoint, import here all the tasks we want to make available
"""

from invoke import Collection

from tasks import (
    agent,
    agentless_scanner,
    bench,
    buildimages,
    cluster_agent,
    cluster_agent_cloudfoundry,
    components,
    cws_instrumentation,
    diff,
    docker_tasks,
    docs,
    dogstatsd,
    ebpf,
    emacs,
    epforwarder,
    fakeintake,
    github_tasks,
    kmt,
    linter,
    modules,
    msi,
    new_e2e_tests,
    notify,
    owners,
    package,
    pipeline,
    process_agent,
    release,
    rtloader,
    security_agent,
    selinux,
    system_probe,
    systray,
    trace_agent,
    updater,
    vscode,
)
from tasks.build_tags import audit_tag_impact, print_default_build_tags
from tasks.components import lint_components, lint_fxutil_oneshot_test
from tasks.fuzz import fuzz
from tasks.go import (
    check_go_mod_replaces,
    check_go_version,
    check_mod_tidy,
    deps,
    deps_vendored,
    generate_licenses,
    generate_protobuf,
    go_fix,
    golangci_lint,
    internal_deps_checker,
    lint_licenses,
    reset,
    tidy_all,
)
from tasks.go_test import (
    codecov,
    e2e_tests,
    get_impacted_packages,
    get_modified_packages,
    integration_tests,
    lint_go,
    send_unit_tests_stats,
    test,
)
from tasks.install_tasks import download_tools, install_shellcheck, install_tools
from tasks.junit_tasks import junit_upload
from tasks.libs.common.go_workspaces import handle_go_work
from tasks.pr_checks import lint_releasenote
from tasks.show_linters_issues import show_linters_issues
from tasks.unit_tests import invoke_unit_tests
from tasks.update_go import go_version, update_go
from tasks.windows_resources import build_messagetable

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
ns.add_task(lint_go)
ns.add_task(lint_fxutil_oneshot_test)
ns.add_task(generate_protobuf)
ns.add_task(reset)
ns.add_task(lint_releasenote)
ns.add_task(show_linters_issues)
ns.add_task(go_version)
ns.add_task(update_go)
ns.add_task(audit_tag_impact)
ns.add_task(print_default_build_tags)
ns.add_task(e2e_tests)
ns.add_task(install_shellcheck)
ns.add_task(download_tools)
ns.add_task(install_tools)
ns.add_task(invoke_unit_tests)
ns.add_task(check_mod_tidy)
ns.add_task(check_go_mod_replaces)
ns.add_task(tidy_all)
ns.add_task(internal_deps_checker)
ns.add_task(check_go_version)
ns.add_task(junit_upload)
ns.add_task(fuzz)
ns.add_task(go_fix)
ns.add_task(build_messagetable)
ns.add_task(get_impacted_packages)

ns.add_task(get_modified_packages)
ns.add_task(send_unit_tests_stats)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(agentless_scanner)
ns.add_collection(buildimages)
ns.add_collection(cluster_agent)
ns.add_collection(cluster_agent_cloudfoundry)
ns.add_collection(components)
ns.add_collection(docs)
ns.add_collection(bench)
ns.add_collection(trace_agent)
ns.add_collection(docker_tasks, "docker")
ns.add_collection(dogstatsd)
ns.add_collection(ebpf)
ns.add_collection(emacs)
ns.add_collection(epforwarder)
ns.add_collection(linter)
ns.add_collection(msi)
ns.add_collection(github_tasks, "github")
ns.add_collection(package)
ns.add_collection(pipeline)
ns.add_collection(notify)
ns.add_collection(selinux)
ns.add_collection(systray)
ns.add_collection(release)
ns.add_collection(rtloader)
ns.add_collection(system_probe)
ns.add_collection(process_agent)
ns.add_collection(security_agent)
ns.add_collection(cws_instrumentation)
ns.add_collection(vscode)
ns.add_collection(new_e2e_tests)
ns.add_collection(fakeintake)
ns.add_collection(kmt)
ns.add_collection(diff)
ns.add_collection(updater)
ns.add_collection(owners)
ns.add_collection(modules)
ns.configure(
    {
        'run': {
            # this should stay, set the encoding explicitly so invoke doesn't
            # freak out if a command outputs unicode chars.
            'encoding': 'utf-8',
        }
    }
)

# disable go workspaces by default
handle_go_work()
