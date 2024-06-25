"""
Emacs namespaced tags

Helpers for getting Emacs set up nicely
"""

from invoke import task

from tasks.build_tags import (
    build_tags,
    filter_incompatible_tags,
    get_build_tags,
    get_default_build_tags,
)
from tasks.flavor import AgentFlavor


@task(
    help={
        "targets": f"Comma separated list of targets to include. Possible values: all, {', '.join(build_tags[AgentFlavor.base].keys())}. Default: all",
        "flavor": f"Agent flavor to use. Possible values: {', '.join(AgentFlavor.__members__.keys())}. Default: {AgentFlavor.base.name}",
    }
)
def set_buildtags(
    _,
    targets="all",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
):
    """
    Create Emacs .dir-locals.el settings file for this project to include correct build tags
    """
    flavor = AgentFlavor[flavor]

    if targets == "all":
        targets = build_tags[flavor].keys()
    else:
        targets = targets.split(",")
        if not set(targets).issubset(build_tags[flavor]):
            print("Must choose valid targets. Valid targets are:")
            print(f'{", ".join(build_tags[flavor].keys())}')
            return

    if build_include is None:
        build_include = []
        for target in targets:
            build_include.extend(get_default_build_tags(build=target, flavor=flavor))
    else:
        build_include = filter_incompatible_tags(build_include.split(","))

    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

    with open(".dir-locals.el", "w") as f:
        f.write(f'((go-mode . ((lsp-go-build-flags . ["-tags", "{",".join(sorted(use_tags))}"])\n')
        f.write(
            '             (eval . (lsp-register-custom-settings \'(("gopls.allowImplicitNetworkAccess" t t)))))))\n'
        )
