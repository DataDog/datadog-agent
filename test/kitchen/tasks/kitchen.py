import glob
import json
import os
import re
import traceback

import requests
from invoke import task
from invoke.exceptions import Exit

WINDOWS_SKIP_IF_TESTSIGNING = ['.*cn']


@task(iterable=['platlist'])
def genconfig(
    ctx,
    platform=None,
    provider=None,
    osversions="all",
    testfiles=None,
    uservars=None,
    platformfile=None,
    platlist=None,
    fips=False,
    arch="x86_64",
    imagesize=None,
):
    """
    Create a kitchen config
    """
    if not platform and not platlist:
        raise Exit(message="Must supply a platform to configure\n", code=1)

    if not testfiles:
        raise Exit(message="Must supply one or more testfiles to include\n", code=1)

    if platlist and (platform or provider):
        raise Exit(
            message="Can specify either a list of specific OS images OR a platform and provider, but not both\n", code=1
        )

    if not platlist and not provider:
        provider = "azure"

    if platformfile:
        with open(platformfile, "r") as f:
            platforms = json.load(f)
    else:
        try:
            print(
                "Fetching the latest kitchen platforms.json from Github. Use --platformfile=platforms.json to override with a local file."
            )
            r = requests.get(
                'https://raw.githubusercontent.com/DataDog/datadog-agent/main/test/kitchen/platforms.json',
                allow_redirects=True,
            )
            r.raise_for_status()
            platforms = r.json()
        except Exception:
            traceback.print_exc()
            print("Warning: Could not fetch the latest kitchen platforms.json from Github, using local version.")
            with open("platforms.json", "r") as f:
                platforms = json.load(f)

    # create the TEST_PLATFORMS environment variable
    testplatformslist = []

    if platform:
        plat = platforms.get(platform)
        if not plat:
            raise Exit(
                message=f"Unknown platform {platform}.  Known platforms are {list(platforms.keys())}\n",
                code=2,
            )

        # check to see if the OS is configured for the given provider
        prov = plat.get(provider)
        if not prov:
            raise Exit(
                message=f"Unknown provider {provider}.  Known providers for platform {platform} are {list(plat.keys())}\n",
                code=3,
            )

        ar = prov.get(arch)
        if not ar:
            raise Exit(
                message=f"Unknown architecture {arch}. Known architectures for platform {platform} provider {provider} are {list(prov.keys())}\n",
                code=4,
            )

        # get list of target OSes
        if osversions.lower() == "all":
            osversions = ".*"

        osimages = load_targets(ctx, ar, osversions, platform)

        print(f"Chose os targets {osimages}\n")
        for osimage in osimages:
            testplatformslist.append(f"{osimage},{ar[osimage]}")

    elif platlist:
        # platform list should be in the form of driver,os,arch,image
        for entry in platlist:
            driver, osv, arch, image = entry.split(",")
            if provider and driver != provider:
                raise Exit(message=f"Can only use one driver type per config ( {provider} != {driver} )\n", code=1)

            provider = driver
            # check to see if we know this one
            if not platforms.get(osv):
                raise Exit(message=f"Unknown OS in {entry}\n", code=4)

            if not platforms[osv].get(driver):
                raise Exit(message=f"Unknown driver in {entry}\n", code=5)

            if not platforms[osv][driver].get(arch):
                raise Exit(message=f"Unknown architecture in {entry}\n", code=5)

            if not platforms[osv][driver][arch].get(image):
                raise Exit(message=f"Unknown image in {entry}\n", code=6)

            testplatformslist.append(f"{image},{platforms[osv][driver][arch][image]}")

    print("Using the following test platform(s)\n")
    for logplat in testplatformslist:
        print(f"  {logplat}")
    testplatforms = "|".join(testplatformslist)

    # create the kitchen.yml file
    with open('tmpkitchen.yml', 'w') as kitchenyml:
        # first read the correct driver
        print(f"Adding driver file drivers/{provider}-driver.yml\n")

        with open(f"drivers/{provider}-driver.yml", 'r') as driverfile:
            kitchenyml.write(driverfile.read())

        # read the generic contents
        with open("test-definitions/platforms-common.yml", 'r') as commonfile:
            kitchenyml.write(commonfile.read())

        # now open the requested test files
        for f in glob.glob(f"test-definitions/{testfiles}.yml"):
            if f.lower().endswith("platforms-common.yml"):
                print("Skipping common file\n")
            with open(f, 'r') as infile:
                print(f"Adding file {f}\n")
                kitchenyml.write(infile.read())

    env = {}
    if uservars:
        env = load_user_env(ctx, provider, uservars)

    # set KITCHEN_ARCH if it's not set in the user env
    if 'KITCHEN_ARCH' not in env and not ('KITCHEN_ARCH' in os.environ.keys()):
        env['KITCHEN_ARCH'] = arch

    env['TEST_PLATFORMS'] = testplatforms

    if provider == "azure":
        env['TEST_IMAGE_SIZE'] = imagesize if imagesize else ""
    elif provider == "ec2" and imagesize:
        env['KITCHEN_EC2_INSTANCE_TYPE'] = imagesize

    if fips:
        env['FIPS'] = 'true'
    ctx.run("erb tmpkitchen.yml > kitchen.yml", env=env)


@task
def should_rerun_failed(_, runlog):
    """
    Parse a log from kitchen run and see if we should rerun it (e.g. because of a network issue).
    """
    test_result_re = re.compile(r'\d+\s+examples?,\s+(?P<failures>\d+)\s+failures?')
    with open(runlog, 'r', encoding='utf-8') as f:
        text = f.read()
        result = set(test_result_re.findall(text))
        if result == {'0'} or result == set():
            print("Seeing no failed tests in log, advising to rerun")
        else:
            raise Exit("Seeing some failed tests in log, not advising to rerun", 1)


def load_targets(_, targethash, selections, platform):
    returnlist = []
    skiplist = []
    commentpattern = re.compile("^comment")

    if platform == "windows":
        if 'WINDOWS_DDNPM_DRIVER' in os.environ.keys() and os.environ['WINDOWS_DDNPM_DRIVER'] == 'testsigned':
            for skip in WINDOWS_SKIP_IF_TESTSIGNING:
                skiplist.append(re.compile(skip))

    for selection in selections.split(","):
        selectionpattern = re.compile(f"^{selection}$")

        matched = False
        for key in targethash:
            if commentpattern.match(key):
                continue
            if selectionpattern.search(key):
                for skip in skiplist:
                    if skip.match(key):
                        print(f"Matched key {key} to skip list, skipping\n")
                        matched = True
                        break
                else:
                    # will only execute if there's not a break in the previous for
                    # loop.
                    matched = True
                    if key not in returnlist:
                        returnlist.append(key)
                    else:
                        print(f"Skipping duplicate target key {key} (matched search {selection})\n")

        if not matched:
            raise Exit(message=f"Couldn't find any match for target {selection}\n", code=7)
    return returnlist


def load_user_env(_, provider, varsfile):
    env = {}
    commentpattern = re.compile("^comment")
    if os.path.exists(varsfile):
        with open(varsfile, "r") as f:
            vars = json.load(f)
            for key, val in vars.get("global", {}).items():
                if commentpattern.match(key):
                    continue
                env[key] = val
            for key, val in vars.get(provider, {}).items():
                if commentpattern.match(key):
                    continue
                env[key] = val
    return env
