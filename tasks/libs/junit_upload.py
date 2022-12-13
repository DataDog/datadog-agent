import glob
import io
import os
import platform
import subprocess
import tarfile
import tempfile
import xml.etree.ElementTree as ET

from invoke.exceptions import Exit

from ..flavor import AgentFlavor
from .pipeline_notifications import DEFAULT_SLACK_CHANNEL, GITHUB_SLACK_MAP

CODEOWNERS_ORG_PREFIX = "@DataDog/"
REPO_NAME_PREFIX = "github.com/DataDog/datadog-agent/"
DATADOG_CI_COMMAND = ["datadog-ci", "junit", "upload"]
JOB_URL_FILE_NAME = "job_url.txt"
TAGS_FILE_NAME = "tags.txt"


def add_flavor_to_junitxml(xml_path: str, flavor: AgentFlavor):
    """
    Takes a JUnit XML file and adds a flavor field to it, to allow tagging
    tests by flavor.
    """
    tree = ET.parse(xml_path)

    # Create a new element containing the flavor and append it to the tree
    flavor_element = ET.Element('flavor')
    flavor_element.text = flavor.name
    tree.getroot().append(flavor_element)

    # Write back to the original file
    tree.write(xml_path)


def split_junitxml(xml_path, codeowners, output_dir):
    """
    Split a junit XML into several according to the suite name and the codeowners.
    Returns a list with the owners of the written files.
    """
    tree = ET.parse(xml_path)
    output_xmls = {}

    flem = tree.find("flavor")
    flavor = flem.text if flem else AgentFlavor.base.name

    for suite in tree.iter("testsuite"):
        path = suite.attrib["name"].replace(REPO_NAME_PREFIX, "", 1)

        # Dirs in CODEOWNERS might end with "/", but testsuite names in JUnit XML
        # don't, so for determining ownership we append "/" temporarily.
        owners = codeowners.of(path + "/")
        if not owners:
            main_owner = "none"
        else:
            main_owner = owners[0][1][len(CODEOWNERS_ORG_PREFIX) :]

        try:
            xml = output_xmls[main_owner]
        except KeyError:
            xml = ET.ElementTree(ET.Element("testsuites"))
            output_xmls[main_owner] = xml
        xml.getroot().append(suite)

    for owner, xml in output_xmls.items():
        filepath = os.path.join(output_dir, owner + ".xml")
        xml.write(filepath, encoding="UTF-8", xml_declaration=True)

    return list(output_xmls), flavor


def upload_junitxmls(output_dir, owners, flavor, additional_tags=None, job_url=""):
    """
    Upload all per-team split JUnit XMLs from given directory.
    """
    processes = []
    process_env = os.environ.copy()
    process_env["CI_JOB_URL"] = job_url
    for owner in owners:
        codeowner = CODEOWNERS_ORG_PREFIX + owner
        slack_channel = GITHUB_SLACK_MAP.get(codeowner, DEFAULT_SLACK_CHANNEL)[1:]
        args = [
            "--service",
            "datadog-agent",
            "--tags",
            f'test.codeowners:["{codeowner}"]',
            "--tags",
            f"test.flavor:{flavor}",
            "--tags",
            f"slack_channel:{slack_channel}",
        ]
        if additional_tags:
            args.extend(additional_tags)
        args.append(os.path.join(output_dir, owner + ".xml"))
        processes.append(subprocess.Popen(DATADOG_CI_COMMAND + args, bufsize=-1, env=process_env))
    for process in processes:
        exit_code = process.wait()
        if exit_code != 0:
            raise subprocess.CalledProcessError(exit_code, DATADOG_CI_COMMAND)


def junit_upload_from_tgz(junit_tgz, codeowners_path=".github/CODEOWNERS"):
    """
    Upload all JUnit XML files contained in given tgz archive.
    """
    from codeowners import CodeOwners

    with open(codeowners_path) as f:
        codeowners = CodeOwners(f.read())

    xmlcounts = {}
    with tempfile.TemporaryDirectory() as unpack_dir:
        # unpack all files from archive
        with tarfile.open(junit_tgz) as tgz:
            tgz.extractall(path=unpack_dir)
        # read additional tags
        tagsfile = os.path.join(unpack_dir, TAGS_FILE_NAME)
        if os.path.exists(tagsfile):
            with open(tagsfile) as tf:
                tags = tf.read().split()
        # read job url (see comment in produce_junit_tar)
        with open(os.path.join(unpack_dir, JOB_URL_FILE_NAME)) as jf:
            job_url = jf.read()

        # for each unpacked xml file, split it and submit all parts
        # NOTE: recursive=True is necessary for "**" to unpack into 0-n dirs, not just 1
        xmls = 0
        for xmlfile in glob.glob(f"{unpack_dir}/**/*.xml", recursive=True):
            xmls += 1
            with tempfile.TemporaryDirectory() as output_dir:
                written_owners, flavor = split_junitxml(xmlfile, codeowners, output_dir)
                upload_junitxmls(output_dir, written_owners, flavor, tags, job_url)
        xmlcounts[junit_tgz] = xmls

    empty_tgzs = []
    for tgz, count in xmlcounts.items():
        print(f"Submitted results for {count} JUnit XML files from {tgz}")
        if count == 0:
            empty_tgzs.append(tgz)

    if empty_tgzs:
        raise Exit(f"No JUnit XML files for upload found in: {', '.join(empty_tgzs)}")


def _normalize_architecture(architecture):
    architecture = architecture.lower()
    normalize_table = {"amd64": "x86_64"}
    return normalize_table.get(architecture, architecture)


def produce_junit_tar(files, result_path):
    """
    Produce a tgz file containing all given files JUnit XML files and add a special file
    with additional tags.
    """
    # NOTE: for now, we can't pass CI_JOB_URL through `--tags`, because
    # the parsing logic tags breaks on URLs, as they contain colons.
    # Therefore we pass it through environment variable.
    tags = {
        "os.platform": platform.system().lower(),
        "os.architecture": _normalize_architecture(platform.machine()),
        "ci.job.name": os.environ.get("CI_JOB_NAME", ""),
        # "ci.job.url": os.environ.get("CI_JOB_URL", ""),
    }
    with tarfile.open(result_path, "w:gz") as tgz:
        for f in files:
            tgz.add(f, arcname=f.replace(os.path.sep, "-"))

        tags_file = io.BytesIO()
        for k, v in tags.items():
            tags_file.write(f"--tags {k}:{v} ".encode("UTF-8"))
        tags_info = tarfile.TarInfo(TAGS_FILE_NAME)
        tags_info.size = tags_file.getbuffer().nbytes
        tags_file.seek(0)
        tgz.addfile(tags_info, tags_file)

        job_url_file = io.BytesIO()
        job_url_file.write(os.environ.get("CI_JOB_URL", "").encode("UTF-8"))
        job_url_info = tarfile.TarInfo(JOB_URL_FILE_NAME)
        job_url_info.size = job_url_file.getbuffer().nbytes
        job_url_file.seek(0)
        tgz.addfile(job_url_info, job_url_file)
