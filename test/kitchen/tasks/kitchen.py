import glob
import json
import os.path
import re

from invoke import task
from invoke.exceptions import Exit


@task(iterable=['platlist'])
def genconfig(
    ctx,
    platform=None,
    provider=None,
    osversions="all",
    testfiles=None,
    uservars=None,
    platformfile="platforms.json",
    platlist=None,
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

    platforms = load_platforms(ctx, platformfile=platformfile)

    # create the TEST_PLATFORMS environment variable
    testplatformslist = []

    if platform:
        plat = platforms.get(platform)
        if not plat:
            raise Exit(
                message="Unknown platform {platform}.  Known platforms are {avail}\n".format(
                    platform=platform, avail=list(platforms.keys())
                ),
                code=2,
            )
            raise Exit(2)

        ## check to see if the OS is configured for the given provider
        prov = plat.get(provider)
        if not prov:
            raise Exit(
                message="Unknown provider {prov}.  Known providers for platform {plat} are {avail}\n".format(
                    prov=provider, plat=platform, avail=list(plat.keys())
                ),
                code=3,
            )

        ## get list of target OSes
        if osversions.lower() == "all":
            osversions = ".*"

        osimages = load_targets(ctx, prov, osversions)

        print("Chose os targets {}\n".format(osimages))
        for osimage in osimages:
            testplatformslist.append("{},{}".format(osimage, prov[osimage]))

    elif platlist:
        # platform list should be in the form of driver,os,image
        for entry in platlist:
            driver, os, image = entry.split(",")
            if provider and driver != provider:
                raise Exit(
                    message="Can only use one driver type per config ( {} != {} )\n".format(provider, driver), code=1
                )

            provider = driver
            # check to see if we know this one
            if not platforms.get(os):
                raise Exit(message="Unknown OS in {}\n".format(entry), code=4)

            if not platforms[os].get(driver):
                raise Exit(message="Unknown driver in {}\n".format(entry), code=5)

            if not platforms[os][driver].get(image):
                raise Exit(message="Unknown image in {}\n".format(entry), code=6)

            testplatformslist.append("{},{}".format(image, platforms[os][driver][image]))

    print("Using the following test platform(s)\n")
    for logplat in testplatformslist:
        print("  {}".format(logplat))
    testplatforms = "|".join(testplatformslist)

    # create the kitchen.yml file
    with open('tmpkitchen.yml', 'w') as kitchenyml:
        # first read the correct driver
        print("Adding driver file drivers/{}-driver.yml\n".format(provider))

        with open("drivers/{}-driver.yml".format(provider), 'r') as driverfile:
            kitchenyml.write(driverfile.read())

        # read the generic contents
        with open("test-definitions/platforms-common.yml", 'r') as commonfile:
            kitchenyml.write(commonfile.read())

        # now open the requested test files
        for f in glob.glob("test-definitions/{}.yml".format(testfiles)):
            if f.lower().endswith("platforms-common.yml"):
                print("Skipping common file\n")
            with open(f, 'r') as infile:
                print("Adding file {}\n".format(f))
                kitchenyml.write(infile.read())

    env = {}
    if uservars:
        env = load_user_env(ctx, provider, uservars)
    env['TEST_PLATFORMS'] = testplatforms
    ctx.run("erb tmpkitchen.yml > kitchen.yml", env=env)


def load_platforms(_, platformfile):
    with open(platformfile, "r") as f:
        platforms = json.load(f)
    return platforms


def load_targets(_, targethash, selections):
    returnlist = []
    commentpattern = re.compile("^comment")
    for selection in selections.split(","):
        selectionpattern = re.compile("^{}$".format(selection))

        for key in targethash:
            if commentpattern.match(key):
                continue
            if selectionpattern.search(key):
                if key not in returnlist:
                    returnlist.append(key)
                else:
                    print("Skipping duplicate target key {} (matched search {})\n".format(key, selection))

    return returnlist


def load_user_env(_, provider, varsfile):
    env = {}
    commentpattern = re.compile("^comment")
    if os.path.exists(varsfile):
        with open("uservars.json", "r") as f:
            vars = json.load(f)
            for key, val in vars['global'].items():
                if commentpattern.match(key):
                    continue
                env[key] = val
            for key, val in vars[provider].items():
                if commentpattern.match(key):
                    continue
                env[key] = val
    return env
