"""
Emacs namespaced tags

Helpers for getting Emacs set up nicely
"""

from invoke import task

from tasks.build_tags import get_build_tags
from tasks.flavor import AgentFlavor


@task
def set_buildtags(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    arch="x64",
):
    """
    Create Emacs .dir-locals.el settings file for this project to include correct build tags
    """
    flavor = AgentFlavor[flavor]

    use_tags = get_build_tags(
        build=target, arch=arch, flavor=flavor, build_include=build_include, build_exclude=build_exclude
    )

    with open(".dir-locals.el", "w") as f:
        f.write(f'((go-mode . ((lsp-go-build-flags . ["-tags", "{",".join(use_tags)}"])\n')
        f.write(
            '             (eval . (lsp-register-custom-settings \'(("gopls.allowImplicitNetworkAccess" t t)))))))\n'
        )
