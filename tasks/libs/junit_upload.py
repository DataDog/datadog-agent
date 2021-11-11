from collections import OrderedDict
import glob
import io
import os.path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import xml.etree.ElementTree as ET

CODEOWNERS_ORG_PREFIX = "@DataDog/"
REPO_NAME_PREFIX = "github.com/DataDog/datadog-agent/"
DATADOG_CI_COMMAND = ["datadog-ci", "junit", "upload"]
TAGS_FILE_NAME = "tags.txt"


def read_codeowners(codeowners_path):
    """
    Read the CODEOWNERS file and return a generator of tuples (path, codeowners).
    The paths are normalized and forced to be relative.
    Only directories are returned and some are blacklisted as not expected to contain tests.
    The codeowners have the org prefix @DataDog stripped away.
    """
    with open(codeowners_path) as file:
        for line in file:
            line = line.strip()
            if not line or line.startswith("#"):
                continue

            try:
                path, owners = line.split(None, 1)
            except TypeError:
                continue

            path = os.path.normpath(path)
            if path.startswith("/"):
                path = path[1:]
            if not os.path.isdir(path):
                continue

            owners = [re.sub(CODEOWNERS_ORG_PREFIX, "", owner, flags=re.IGNORECASE) for owner in owners.split()]
            yield path, owners


def split_junitxml(xml_path, codeowners, output_dir):
    """
    Split a junit XML into several according to the suite name and the codeowners.
    Returns a list with the owners of the written files.
    """
    tree = ET.parse(xml_path)
    output_xmls = {}
    for suite in tree.iter("testsuite"):
        path = suite.attrib["name"].replace(REPO_NAME_PREFIX, "", 1)

        owners = None
        try:
            owners = codeowners[path]
        except KeyError:
            for owned_path, path_owners in codeowners.items():
                if path.startswith(owned_path):
                    owners = path_owners

        if owners is None:
            raise KeyError("No code owner found for {}".format(path))

        main_owner = owners[0]
        try:
            xml = output_xmls[main_owner]
        except KeyError:
            xml = ET.ElementTree(ET.Element("testsuites"))
            output_xmls[main_owner] = xml
        xml.getroot().append(suite)

    for owner, xml in output_xmls.items():
        filepath = os.path.join(output_dir, owner + ".xml")
        xml.write(filepath, encoding="UTF-8", xml_declaration=True)

    return list(output_xmls)


def upload_junitxmls(output_dir, owners, additional_tags=None):
    """
    Upload all per-team split JUnit XMLs from given directory.
    """
    processes = []
    for owner in owners:
        args = [
            "--service",
            "datadog-agent",
            "--tags",
            'test.codeowners:["' + CODEOWNERS_ORG_PREFIX + owner + '"]',
        ]
        if additional_tags:
            args.extend(additional_tags)
        args.append(os.path.join(output_dir, owner + ".xml"))
        processes.append(subprocess.Popen(DATADOG_CI_COMMAND + args, bufsize=-1))
    for process in processes:
        exit_code = process.wait()
        if exit_code != 0:
            raise subprocess.CalledProcessError(exit_code, DATADOG_CI_COMMAND)


def junit_upload_from_tgz(junit_tgz, codeowners_path=".github/CODEOWNERS"):
    """
    Upload all JUnit XML files contained in given tgz archive.
    """
    codeowners = OrderedDict(list(read_codeowners(codeowners_path)))
    with tempfile.TemporaryDirectory() as unpack_dir:
        # unpack all files from archive
        with tarfile.open(junit_tgz) as tgz:
            tgz.extractall(path=unpack_dir)
        # read additional tags
        with open(os.path.join(unpack_dir, TAGS_FILE_NAME)) as tf:
            tags = tf.read().split()

        # for each unpacked xml file, split it and submit all parts
        for xmlfile in glob.glob("{}/*.xml".format(unpack_dir)):
            with tempfile.TemporaryDirectory() as output_dir:
                written_owners = split_junitxml(xmlfile, codeowners, output_dir)
                upload_junitxmls(output_dir, written_owners, tags)


def produce_junit_tar(files, result_path):
    """
    Produce a tgz file containing all given files JUnit XML files and add a special file
    with additional tags.
    """
    tags = {
        "os.platform": platform.system().lower(),
        "os.architecture": platform.machine(),
    }
    with tarfile.open(result_path, "w:gz") as tgz:
        for f in files:
            tgz.add(f, arcname=f.replace(os.path.sep, "-"))
        tags_file = io.BytesIO()
        for k, v in tags.items():
            tags_file.write("--tags {}:{} ".format(k, v).encode("UTF-8"))
        tags_info = tarfile.TarInfo(TAGS_FILE_NAME)
        tags_info.size = tags_file.getbuffer().nbytes
        tags_file.seek(0)
        tgz.addfile(tags_info, tags_file)
