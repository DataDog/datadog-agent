"""
Utility functions for manipulating licenses
"""

import json
import os
import re
import shutil
import tempfile
import textwrap
import time

import yaml
from invoke.exceptions import Exit
from invoke import task
from types import SimpleNamespace
from contextlib import contextmanager

# Files searched for COPYRIGHT_RE
COPYRIGHT_LOCATIONS = [
    'license',
    'LICENSE',
    'license.md',
    'LICENSE.md',
    'LICENSE.txt',
    'License.txt',
    'license.txt',
    'COPYING',
    'NOTICE',
    'README',
    'README.md',
    'README.mdown',
    'README.markdown',
    'COPYRIGHT',
    'COPYRIGHT.txt',
]

AUTHORS_LOCATIONS = [
    'AUTHORS',
    'AUTHORS.md',
    'CONTRIBUTORS',
]

# General match for anything that looks like a copyright declaration
COPYRIGHT_RE = re.compile(r'copyright\s+(?:©|\(c\)\s+)?(?:(?:[0-9 ,-]|present)+\s+)?(?:by\s+)?(.*)', re.I)

# Copyright strings to ignore, as they are not owners.  Most of these are from
# boilerplate license files.
#
# These match at the beginning of the copyright (the result of COPYRIGHT_RE).
COPYRIGHT_IGNORE_RES = [
    re.compile(r'copyright(:? and license)?$', re.I),
    re.compile(r'copyright (:?holder|owner|notice|license|statement)', re.I),
    re.compile(r'Copyright & License -'),
    re.compile(r'copyright .yyyy. .name of copyright owner.', re.I),
    re.compile(r'copyright .yyyy. .name of copyright owner.', re.I),
]

# Match for various suffixes that need not be included
STRIP_SUFFIXES_RE = []

# Packages containing CONTRIBUTORS files that do not use #-style comments
# in their header; we skip until the first blank line.
CONTRIBUTORS_WITH_UNCOMMENTED_HEADER = [
    'github.com/patrickmn/go-cache',
    'gopkg.in/Knetic/govaluate.v3',
]


# FIXME: This doesn't include licenses for non-go dependencies, like the javascript libs we use for the web gui
def get_licenses_list(ctx, licenses_filename='LICENSE-3rdparty.csv'):
    deps_vendored(ctx)

    try:
        licenses = wwhrd_licenses(ctx)
        licenses = find_copyright(ctx, licenses)
        licenses = licenses_csv(licenses)
        _verify_unknown_licenses(licenses, licenses_filename)

        return licenses
    finally:
        shutil.rmtree("vendor/")


def _verify_unknown_licenses(licenses, licenses_filename):
    """
    Check that all deps have a non-"UNKNOWN" copyright and license
    """
    unknown_licenses = False
    for line in licenses:
        parts = [part.strip().casefold() for part in line.split(',')]
        if 'unknown' in parts:
            unknown_licenses = True
            print(f"! {line}")

    if unknown_licenses:
        raise Exit(
            message=textwrap.dedent(
                """\
                At least one dependency's license or copyright could not be determined.

                Consult the dependency's source, update
                `.copyright-overrides.yml` or `.wwhrd.yml` accordingly, and run
                `inv generate-licenses` to update {}."""
            ).format(licenses_filename),
            code=1,
        )


def is_valid_quote(copyright):
    stack = []
    quotes_to_check = ['"']
    for c in copyright:
        if c in quotes_to_check:
            if stack and stack[-1] == c:
                stack.pop()
            else:
                stack.append(c)
    return len(stack) == 0


def licenses_csv(licenses):
    licenses.sort(key=lambda lic: lic["package"])

    def fmt_copyright(lic):
        # discards copyright with invalid quotes to ensure generated csv is valid
        filtered_copyright = []
        for copyright in lic["copyright"]:
            if is_valid_quote(copyright):
                filtered_copyright.append(copyright)
            else:
                print(
                    f'The copyright `{copyright}` of `{lic["component"]},{lic["package"]}` was discarded because its copyright contains invalid quotes. To fix the discarded copyright, modify `.copyright-overrides.yml` to fix the bad-quotes copyright'
                )
        if len(copyright) == 0:
            copyright = "UNKNOWN"
        copyright = ' | '.join(sorted(filtered_copyright))
        # quote for inclusion in CSV, if necessary
        if ',' in copyright:
            copyright = copyright.replace('"', '""')
            copyright = f'"{copyright}"'
        return copyright

    return [
        f"{license['component']},{license['package']},{license['license']},{fmt_copyright(license)}"
        for license in licenses
    ]


def wwhrd_licenses(ctx):
    # local imports
    from urllib.parse import urlparse

    import requests
    from requests.exceptions import RequestException

    # Read the list of packages to exclude from the list from wwhrd's
    exceptions_wildcard = []
    exceptions = []
    additional = {}
    overrides = {}
    with open('.wwhrd.yml', encoding="utf-8") as wwhrd_conf_yml:
        wwhrd_conf = yaml.safe_load(wwhrd_conf_yml)
        for pkg in wwhrd_conf['exceptions']:
            if pkg.endswith("/..."):
                # TODO(python3.9): use removesuffix
                exceptions_wildcard.append(pkg[: -len("/...")])
            else:
                exceptions.append(pkg)

        for pkg, license in wwhrd_conf.get('additional', {}).items():
            additional[pkg] = license

        for pkg, lic in wwhrd_conf.get('overrides', {}).items():
            overrides[pkg] = lic

    def is_excluded(pkg):
        if pkg in exceptions:
            return True
        for exception in exceptions_wildcard:
            if pkg.startswith(exception):
                return True
        return False

    # Parse the output of wwhrd to generate the list
    result = ctx.run('wwhrd list --no-color', hide='err')
    licenses = []
    if result.stderr:
        for line in result.stderr.split("\n"):
            index = line.find('msg="Found License"')
            if index == -1:
                continue
            license = ""
            package = ""
            for val in line[index + len('msg="Found License"') :].split(" "):
                if val.startswith('license='):
                    license = val[len('license=') :]
                elif val.startswith('package='):
                    package = val[len('package=') :]
                    if is_excluded(package):
                        print(f"Skipping {package} ({license}) excluded in .wwhrd.yml")
                    else:
                        if package in overrides:
                            license = overrides[package]
                        licenses.append({"component": "core", "package": package, "license": license})

    for pkg, lic in additional.items():
        url = urlparse(lic)
        url = url._replace(scheme='https', netloc=url.path, path='')
        try:
            resp = requests.get(url.geturl())
            resp.raise_for_status()

            with tempfile.TemporaryDirectory() as tempdir:
                with open(os.path.join(tempdir, 'LICENSE'), 'w', encoding="utf-8") as lfp:
                    lfp.write(resp.text)
                    lfp.flush()

                    temp_path = os.path.dirname(lfp.name)
                    result = ctx.run(f"license-detector -f json {temp_path}", hide="out")
                    if result.stdout:
                        results = json.loads(result.stdout)
                        for project in results:
                            if 'error' in project:
                                continue

                            # we get the first match
                            license = project['matches'][0]['license']
                        licenses.append({"component": "core", "package": pkg, "license": license})
        except RequestException as e:
            print(f"There was an issue reaching license {pkg} for pkg {lic}")
            raise Exit(code=1) from e

    return licenses


def find_copyright_for(package, overrides, ctx):
    copyright = []

    over = overrides(package)
    if over:
        return over

    # since this is a package path, copyright information for the go module may
    # be in a parent directory.
    if package.count('/') > 0:
        parent = find_copyright_for('/'.join(package.split('/')[:-1]), overrides, ctx)
    else:
        parent = []

    # search the package dir for a bunch of heuristically-useful files that might
    # contain copyright or authorship information
    pkgdir = os.path.join('vendor', package)

    for filename in COPYRIGHT_LOCATIONS:
        filename = os.path.join(pkgdir, filename)
        if os.path.isfile(filename):
            with open(filename, encoding="utf-8", errors="replace") as f:
                lines = f.readlines()

            for line in lines:
                mo = COPYRIGHT_RE.search(line)
                if not mo:
                    continue
                cpy = mo.group(0)

                # ignore a few spurious matches from license boilerplate
                if any(ign.match(cpy) for ign in COPYRIGHT_IGNORE_RES):
                    continue

                # strip some suffixes
                for suff_re in STRIP_SUFFIXES_RE:
                    cpy = suff_re.sub('', cpy)

                cpy = cpy.strip().rstrip('.')
                if cpy:
                    # If copyright contains double quote ("), escape it
                    if '"' in cpy:
                        cpy = '"' + cpy.replace('"', '""') + '"'
                    copyright.append(cpy)

    # skip through the first blank line of a file
    def skipheader(lines):
        for line in lines:
            if not line.strip():
                break
        for line in lines:
            yield line

    for filename in AUTHORS_LOCATIONS:
        filename = os.path.join(pkgdir, filename)
        if os.path.isfile(filename):
            lines = open(filename, encoding="utf-8")
            if package in CONTRIBUTORS_WITH_UNCOMMENTED_HEADER:
                lines = skipheader(lines)
            for line in lines:
                line = line.strip()
                if not line or line[0] == '#':
                    continue
                copyright.append(line)

    return list(set(parent + copyright))


def read_overrides():
    with open('.copyright-overrides.yml', encoding='utf-8') as overrides_yml:
        override_spec = yaml.safe_load(overrides_yml)
    override_pats = []
    for pkg, dpy in override_spec.items():
        # cast dpy to a list
        if not isinstance(dpy, list):
            dpy = [dpy]
            override_spec[pkg] = dpy

        if pkg.endswith('*'):
            pkg = pkg[:-1]
            override_pats.append((pkg, dpy))

    def overrides(pkg):
        try:
            return override_spec[pkg]
        except KeyError:
            pass

        for pat, dpy in override_pats:
            if pkg.startswith(pat):
                return dpy

    return overrides


def find_copyright(ctx, licenses):
    overrides = read_overrides()
    for lic in licenses:
        pkg = lic['package']
        cpy = find_copyright_for(pkg, overrides, ctx)
        if cpy:
            lic['copyright'] = cpy
        else:
            lic['copyright'] = ['UNKNOWN']

    return licenses

@task
def lint_licenses(ctx):
    """
    Checks that the LICENSE-3rdparty.csv file is up-to-date with contents of go.sum
    """
    print("Verify licenses")

    licenses = []
    file = 'LICENSE-3rdparty.csv'
    with open(file, encoding='utf-8') as f:
        next(f)
        for line in f:
            licenses.append(line.rstrip())

    new_licenses = get_licenses_list(ctx, file)

    removed_licenses = [ele for ele in new_licenses if ele not in licenses]
    for license in removed_licenses:
        print(f"+ {license}")

    added_licenses = [ele for ele in licenses if ele not in new_licenses]
    for license in added_licenses:
        print(f"- {license}")

    if len(removed_licenses) + len(added_licenses) > 0:
        raise Exit(
            message=textwrap.dedent(
                """\
                Licenses are not up-to-date.

                Please run 'inv generate-licenses' to update {}."""
            ).format(file),
            code=1,
        )

    print("Licenses are ok.")


@task
def generate_licenses(ctx, filename='LICENSE-3rdparty.csv', verbose=False):
    """
    Generates the LICENSE-3rdparty.csv file. Run this if `inv lint-licenses` fails.
    """
    new_licenses = get_licenses_list(ctx, filename)

    with open(filename, 'w') as f:
        f.write("Component,Origin,License,Copyright\n")
        for license in new_licenses:
            if verbose:
                print(license)
            f.write(f'{license}\n')
    print("licenses files generated")

@task
def deps_vendored(ctx, verbose=False):
    """
    Vendor Go dependencies
    """

    print("vendoring dependencies")
    with timed("go mod vendor"):
        verbosity = ' -v' if verbose else ''

        # We need to set GOWORK=off to avoid the go command to use the go.work directory
        # It is needed because it does not work very well with vendoring, we should no longer need it when we get rid of vendoring. ADXR-766
        ctx.run(f"go mod vendor{verbosity}", env={"GOWORK": "off"})
        ctx.run(f"go mod tidy{verbosity}", env={"GOWORK": "off"})

        # "go mod vendor" doesn't copy files that aren't in a package: https://github.com/golang/go/issues/26366
        # This breaks when deps include other files that are needed (eg: .java files from gomobile): https://github.com/golang/go/issues/43736
        # For this reason, we need to use a 3rd party tool to copy these files.
        # We won't need this if/when we change to non-vendored modules
        ctx.run(f'modvendor -copy="**/*.c **/*.h **/*.proto **/*.java"{verbosity}')

        # If github.com/DataDog/datadog-agent gets vendored too - nuke it
        # This may happen because of the introduction of nested modules
        if os.path.exists('vendor/github.com/DataDog/datadog-agent'):
            print("Removing vendored github.com/DataDog/datadog-agent")
            shutil.rmtree('vendor/github.com/DataDog/datadog-agent')

@contextmanager
def timed(name="", quiet=False):
    """Context manager that prints how long it took"""
    start = time.time()
    res = SimpleNamespace()
    print(f"{name}")
    try:
        yield res
    finally:
        res.duration = time.time() - start
        if not quiet:
            print(f"{name} completed in {res.duration:.2f}s")
