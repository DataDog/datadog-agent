"""
Docker related tasks
"""
from __future__ import print_function, absolute_import
import tempfile
import shutil
import sys
import os
import re
import time
import yaml

from invoke import task
from invoke.exceptions import Exit

from .dogstatsd import DOGSTATSD_TAG

def retry_run(ctx, *args, **kwargs):
    remaining_retries = 5
    while True:
        warn = True
        if remaining_retries == 0:
            warn = False

        r = ctx.run(*args, warn=warn, **kwargs)

        if r.ok:
            return r

        # Pause between retries. Hope it helps.
        time.sleep(5)

        remaining_retries -= 1

    return r

@task
def test(ctx):
    """
    Run docker tests
    """
    ctx.run("python3 ./Dockerfiles/agent/secrets-helper/test_readsecret.py")


@task
def integration_tests(ctx, skip_image_build=False, skip_build=False):
    """
    Run docker integration tests
    """
    if not skip_image_build:
        # postpone the import otherwise `image_build` will be added to the docker
        # namespace
        from .dogstatsd import image_build
        image_build(ctx, skip_build=skip_build)

    print("Starting docker integration tests")
    env = {"DOCKER_IMAGE": DOGSTATSD_TAG}
    ctx.run("python ./test/integration/docker/dsd_listening.py", env=env)


@task
def dockerize_test(ctx, binary, skip_cleanup=False):
    """
    Run a go test in a remote docker environment and pipe its output to stdout.
    Host and target systems must be identical (test is built on the host).
    """
    import docker

    client = docker.from_env()
    temp_folder = tempfile.mkdtemp(prefix="ddtest-")

    ctx.run("cp %s %s/test.bin" % (binary, temp_folder))

    with open("%s/Dockerfile" % temp_folder, 'w') as stream:
        stream.write("""FROM debian:stretch-slim
ENV DOCKER_DD_AGENT=yes
WORKDIR /
ADD https://github.com/docker/compose/releases/download/1.16.1/docker-compose-Linux-x86_64 /bin/docker-compose
RUN echo "1804b0ce6596efe707b9cab05d74b161833ed503f0535a937dd5d17bea8fc50a  /bin/docker-compose" > sum && \
    sha256sum -c sum && \
    chmod +x /bin/docker-compose
CMD /test.bin
COPY test.bin /test.bin
""")
        # Handle optional testdata folder
        if os.path.isdir("./testdata"):
            ctx.run("cp -R testdata %s" % temp_folder)
            stream.write("COPY testdata /testdata")

    test_image, _ = client.images.build(path=temp_folder, rm=True)

    scratch_volume = client.volumes.create()

    test_container = client.containers.run(
        test_image.id,
        detach=True,
        pid_mode="host",  # For origin detection
        environment=[
            "SCRATCH_VOLUME_NAME=" + scratch_volume.name,
            "SCRATCH_VOLUME_PATH=/tmp/scratch",
        ],
        volumes={'/var/run/docker.sock': {'bind': '/var/run/docker.sock', 'mode': 'ro'},
                 '/proc': {'bind': '/host/proc', 'mode': 'ro'},
                 '/sys/fs/cgroup': {'bind': '/host/sys/fs/cgroup', 'mode': 'ro'},
                 scratch_volume.name: {'bind': '/tmp/scratch', 'mode': 'rw'}})

    exit_code = test_container.wait()['StatusCode']

    print(test_container.logs(
        stdout=True,
        stderr=False,
        stream=False))

    sys.stderr.write(test_container.logs(
        stdout=False,
        stderr=True,
        stream=False))

    if not skip_cleanup:
        shutil.rmtree(temp_folder)
        test_container.remove(v=True, force=True)
        scratch_volume.remove(force=True)
        client.images.remove(test_image.id)

    if exit_code != 0:
        raise Exit(code=exit_code)


@task
def mirror_image(ctx, src_image, dst_image="datadog/docker-library", dst_tag="auto"):
    """
    Pull an upstream image and mirror it to our docker-library repository
    for integration tests. Tag format should be A-Z_n_n_n
    """
    if dst_tag == "auto":
        # Autogenerate tag
        match = re.search(r'([^:\/\s]+):[v]?(.*)$', src_image)
        if not match:
            print("Cannot guess destination tag for {}, please provide a --dst-tag option".format(src_image))
            raise Exit(code=1)
        dst_tag = "_".join(match.groups()).replace(".", "_")

    dst = "{}:{}".format(dst_image, dst_tag)
    publish(src_image, dst)


@task
def publish(ctx, src, dst, signed_pull=False, signed_push=False):
    print("Uploading {} to {}".format(src, dst))

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"
    ctx.run(
        "docker pull {src} && docker tag {src} {dst}".format(src=src, dst=dst),
        env=pull_env
    )

    push_env = {}
    if signed_push:
        push_env["DOCKER_CONTENT_TRUST"] = "1"
    retry_run(ctx, "docker push {dst}".format(dst=dst),
              env=push_env,
    )

    ctx.run("docker rmi {src} {dst}".format(src=src, dst=dst))


@task(iterable=['platform'])
def publish_bulk(ctx, platform, src_template, dst_template, signed_push=False):
    """
    Publish a group of platform-specific images.
    """
    for p in platform:
        parts = p.split("/")

        if len(parts) != 2:
            print("Invalid platform format: expected 'OS/ARCH' parameter, got {}".format(p))
            raise Exit(code=1)

        def evalTemplate(s):
            s = s.replace("OS", parts[0].lower())
            s = s.replace("ARCH", parts[1].lower())
            return s

        publish(ctx, evalTemplate(src_template), evalTemplate(dst_template), signed_push=signed_push)

@task(iterable=['image'])
def publish_manifest(ctx, name, tag, image, signed_push=False):
    """
    Publish a manifest referencing image names matching the specified pattern.
    In that pattern, OS and ARCH strings are replaced, if found, by corresponding
    entries in the list of platforms passed as an argument. This allows creating
    a set of image references more easily. See the manifest tool documentation for
    further details: https://github.com/estesp/manifest-tool.
    """
    manifest_spec = {
        "image": "{}:{}".format(name, tag)
    }
    src_images = []

    for img in image:
        img_splitted = img.replace(' ', '').split(',')
        if len(img_splitted) != 2:
            print("Impossible to parse source format for: '{}'".format(img))
            raise Exit(code=1)

        platform_splitted = img_splitted[1].split('/')
        if len(platform_splitted) != 2:
            print("Impossible to parse platform format for: '{}'".format(img))
            raise Exit(code=1)

        src_images.append({
            "image": img_splitted[0],
            "platform": {
                "architecture": platform_splitted[1],
                "os": platform_splitted[0]
            }
        })
    manifest_spec["manifests"] = src_images

    with tempfile.NamedTemporaryFile(mode='w', delete=False) as f:
        temp_file_path = f.name
        yaml.dump(manifest_spec, f, default_flow_style=False)

    print("Using temp file: {}".format(temp_file_path))
    ctx.run("cat {}".format(temp_file_path))

    try:
        result = retry_run(ctx, "manifest-tool push from-spec {}".format(temp_file_path))
        if result.stdout:
            out = result.stdout.split('\n')[0]
            fields = out.split(" ")

            if len(fields) != 3:
                print("Unexpected output when invoking manifest-tool")
                raise Exit(code=1)

            digest_fields = fields[1].split(":")

            if len(digest_fields) != 2 or digest_fields[0] != "sha256":
                print("Unexpected digest format in manifest-tool output")
                raise Exit(code=1)

            digest = digest_fields[1]
            length = fields[2]

        if signed_push:
            cmd = """
            notary -s https://notary.docker.io -d {home}/.docker/trust addhash \
                -p docker.io/{name} {tag} {length} --sha256 {sha256} \
                -r targets/releases
            """
            retry_run(ctx, cmd.format(home=os.path.expanduser("~"), name=name, tag=tag, length=length, sha256=digest))
    finally:
        os.remove(temp_file_path)

@task
def delete(ctx, org, image, tag, token):
    print("Deleting {org}/{image}:{tag}".format(org=org, image=image, tag=tag))
    ctx.run("curl 'https://hub.docker.com/v2/repositories/{org}/{image}/tags/{tag}/' -X DELETE -H 'Authorization: JWT {token}' &>/dev/null".format(org=org, image=image, tag=tag, token=token))

@task
def pull_base_images(ctx, dockerfile, signed_pull=True):
    """
    Pulls the base images for a given Dockerfile, with
    content trust enabled by default, to ensure the base
    images are signed
    """
    images = set()
    stages = set()

    with open(dockerfile, "r") as f:
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
        print("Ignoring intermediate stage names: {}".format(", ".join(stages)))
        images -= stages

    print("Pulling following base images: {}".format(", ".join(images)))

    pull_env = {}
    if signed_pull:
        pull_env["DOCKER_CONTENT_TRUST"] = "1"

    for i in images:
        ctx.run("docker pull {}".format(i), env=pull_env)
