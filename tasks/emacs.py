"""
Emacs namespaced tags

Helpers for getting Emacs set up nicely
"""

from invoke import task

from tasks.build_tags import build_tags, filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor


@task
def set_buildtags(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
):
    """
    Create Emacs .dir-locals.el settings file for this project to include correct build tags
    """
    flavor = AgentFlavor[flavor]

    if target not in build_tags[flavor]:
        print("Must choose a valid target.  Valid targets are: \n")
        print(f'{", ".join(build_tags[flavor].keys())} \n')
        return

    build_include = (
        get_default_build_tags(build=target, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

    with open(".dir-locals.el", "w") as f:
        f.write(f'((go-mode . ((lsp-go-build-flags . ["-tags", "{",".join(use_tags)}"])\n')
        f.write(
            '             (eval . (lsp-register-custom-settings \'(("gopls.allowImplicitNetworkAccess" t t)))))))\n'
        )
