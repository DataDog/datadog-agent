"""
Docker related tasks
"""

from invoke import task


@task
def test(ctx):
    """
    Run docker tests
    """
    ctx.run("python3 ./Dockerfiles/agent/secrets-helper/test_readsecret.py")


@task
def delete(ctx, org, image, tag, token):
    print(f"Deleting {org}/{image}:{tag}")
    ctx.run(
        f"curl 'https://hub.docker.com/v2/repositories/{org}/{image}/tags/{tag}/' -X DELETE -H 'Authorization: JWT {token}' &>/dev/null"
    )


@task
def pull_base_images(ctx, dockerfile, signed_pull=True):
    """
    Pulls the base images for a given Dockerfile, with
    content trust enabled by default, to ensure the base
    images are signed
    """
    images = set()
    stages = set()

    with open(dockerfile) as f:
        for line in f:
            words = line.split()
            # Get source images
            if len(words) < 2 or words[0].lower() != "from":
                continue
            images.add(words[1])
            # Get stage names to remove them from pull
            if len(words) < 4 or words[2].lower() != "as":
                continue
            stages.add(words[3])

    if stages:
        print(f"Ignoring intermediate stage names: {', '.join(stages)}")
        images -= stages

    print(f"Pulling following base images: {', '.join(images)} (content-trust:{signed_pull})")

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"

    for i in images:
        ctx.run(f"docker pull {i}", env=pull_env)
