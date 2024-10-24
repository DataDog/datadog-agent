# https://github.com/pyinvoke/invoke/issues/946
# mypy: disable-error-code="arg-type"

"""
Invoke entrypoint, import here all the tasks we want to make available
"""

from invoke import Collection, Task

from tasks import (
    agent,
    ami,
    bench,
    buildimages,
    cluster_agent,
    cluster_agent_cloudfoundry,
    collector,
    components,
    coverage,
    cws_instrumentation,
    debug,
    debugging,
    devcontainer,
    diff,
    docker_tasks,
    docs,
    dogstatsd,
    ebpf,
    emacs,
    epforwarder,
    fakeintake,
    git,
    github_tasks,
    gitlab_helpers,
    go_deps,
    installer,
    invoke_unit_tests,
    kmt,
    linter,
    modules,
    msi,
    new_e2e_tests,
    notes,
    notify,
    omnibus,
    oracle,
    otel_agent,
    owners,
    package,
    pipeline,
    pre_commit,
    process_agent,
    release,
    rtloader,
    sds,
    security_agent,
    selinux,
    setup,
    system_probe,
    systray,
    testwasher,
    trace_agent,
    vim,
    vscode,
)
from tasks.build_tags import audit_tag_impact, print_default_build_tags
from tasks.components import lint_components, lint_fxutil_oneshot_test
from tasks.custom_task.custom_task import custom__call__
from tasks.fuzz import fuzz
from tasks.go import (
    check_go_mod_replaces,
    check_go_version,
    check_mod_tidy,
    create_module,
    deps,
    deps_vendored,
    generate_licenses,
    generate_protobuf,
    go_fix,
    internal_deps_checker,
    lint_licenses,
    mod_diffs,
    reset,
    tidy,
    tidy_all,
)
from tasks.gotest import (
    check_otel_build,
    check_otel_module_versions,
    e2e_tests,
    get_impacted_packages,
    get_modified_packages,
    integration_tests,
    lint_go,
    send_unit_tests_stats,
    test,
)
from tasks.install_tasks import (
    download_tools,
    install_devcontainer_cli,
    install_protoc,
    install_shellcheck,
    install_tools,
)
from tasks.junit_tasks import junit_upload
from tasks.libs.common.go_workspaces import handle_go_work
from tasks.show_linters_issues.show_linters_issues import show_linters_issues
from tasks.update_go import go_version, update_go
from tasks.windows_resources import build_messagetable

Task.__call__ = custom__call__

# the root namespace
ns = Collection()

# add single tasks to the root
ns.add_task(test)
ns.add_task(integration_tests)
ns.add_task(deps)
ns.add_task(deps_vendored)
ns.add_task(lint_licenses)
ns.add_task(generate_licenses)
ns.add_task(lint_components)
ns.add_task(lint_fxutil_oneshot_test)
ns.add_task(generate_protobuf)
ns.add_task(reset)
ns.add_task(show_linters_issues)
ns.add_task(go_version)
ns.add_task(update_go)
ns.add_task(audit_tag_impact)
ns.add_task(print_default_build_tags)
ns.add_task(e2e_tests)
ns.add_task(install_shellcheck)
ns.add_task(install_protoc)
ns.add_task(install_devcontainer_cli)
ns.add_task(download_tools)
ns.add_task(install_tools)
ns.add_task(check_mod_tidy)
ns.add_task(check_go_mod_replaces)
ns.add_task(check_otel_build)
ns.add_task(check_otel_module_versions)
ns.add_task(tidy)
ns.add_task(tidy_all)
ns.add_task(internal_deps_checker)
ns.add_task(check_go_version)
ns.add_task(create_module)
ns.add_task(junit_upload)
ns.add_task(fuzz)
ns.add_task(go_fix)
ns.add_task(build_messagetable)
ns.add_task(get_impacted_packages)
ns.add_task(get_modified_packages)
ns.add_task(send_unit_tests_stats)
ns.add_task(mod_diffs)
# To deprecate
ns.add_task(lint_go)

# add namespaced tasks to the root
ns.add_collection(agent)
ns.add_collection(ami)
ns.add_collection(buildimages)
ns.add_collection(cluster_agent)
ns.add_collection(cluster_agent_cloudfoundry)
ns.add_collection(components)
ns.add_collection(coverage)
ns.add_collection(debugging)
ns.add_collection(docs)
ns.add_collection(bench)
ns.add_collection(trace_agent)
ns.add_collection(docker_tasks, "docker")
ns.add_collection(dogstatsd)
ns.add_collection(ebpf)
ns.add_collection(emacs)
ns.add_collection(vim)
ns.add_collection(epforwarder)
ns.add_collection(go_deps)
ns.add_collection(linter)
ns.add_collection(msi)
ns.add_collection(git)
ns.add_collection(github_tasks, "github")
ns.add_collection(gitlab_helpers, "gitlab")
ns.add_collection(package)
ns.add_collection(pipeline)
ns.add_collection(notes)
ns.add_collection(notify)
ns.add_collection(oracle)
ns.add_collection(otel_agent)
ns.add_collection(sds)
ns.add_collection(selinux)
ns.add_collection(setup)
ns.add_collection(systray)
ns.add_collection(release)
ns.add_collection(rtloader)
ns.add_collection(system_probe)
ns.add_collection(process_agent)
ns.add_collection(testwasher)
ns.add_collection(security_agent)
ns.add_collection(cws_instrumentation)
ns.add_collection(vscode)
ns.add_collection(new_e2e_tests)
ns.add_collection(fakeintake)
ns.add_collection(kmt)
ns.add_collection(diff)
ns.add_collection(installer)
ns.add_collection(owners)
ns.add_collection(modules)
ns.add_collection(pre_commit)
ns.add_collection(devcontainer)
ns.add_collection(omnibus)
ns.add_collection(collector)
ns.add_collection(invoke_unit_tests)
ns.add_collection(debug)
ns.configure(
    {
        "run": {
            # this should stay, set the encoding explicitly so invoke doesn't
            # freak out if a command outputs unicode chars.
            "encoding": "utf-8",
        }
    }
)

# disable go workspaces by default
handle_go_work()
