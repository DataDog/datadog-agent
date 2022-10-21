"""
Invoke entrypoint, import here all the tasks we want to make available
"""
from collections import namedtuple

from invoke import task
from invoke.exceptions import Exit

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
