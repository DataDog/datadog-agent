import os
if not os.name == 'nt':
    import pwd
else:
    import win32security
import importlib.metadata
import packaging
import shutil
import subprocess
import tempfile
import time

import packaging.requirements
import packaging.version

DO_NOT_REMOVE_WARNING_HEADER = "# DO NOT REMOVE/MODIFY - used internally by installation process\n"


class IntegrationInstallError(Exception):
    """Raised when a single package install fails after all retries."""
    def __init__(self, package, returncode, stderr):
        self.package = package
        self.returncode = returncode
        self.stderr = stderr
        super().__init__(f"failed to install '{package}' (exit {returncode}): {stderr.strip()}")


class IntegrationsRestoreError(Exception):
    """Raised when one or more packages could not be restored during an upgrade."""
    def __init__(self, failures):
        self.failures = failures  # list[IntegrationInstallError]
        names = ", ".join(e.package for e in failures)
        super().__init__(f"failed to restore {len(failures)} package(s): {names}")

# List of PyPi package that start with datadog- prefix but that are datadog integrations
DEPS_STARTING_WITH_DATADOG = [
    "datadog-a7",
    "datadog-agent-dev",
    "datadog-api-client",
    "datadog-api-client-python",
    "datadog-ariadne-graphql-server",
    "datadog-cdk-constructs",
    "datadog-cdk-constructs-v2",
    "datadog-checks-base",
    "datadog-checks-dev",
    "datadog-checks-downloader",
    "datadog-cli",
    "datadog-custom-logger",
    "datadog-dashboard-deployer",
    "datadog-deployer",
    "datadog-export",
    "datadog-exporter",
    "datadog-google-openid",
    "datadog-healthcheck-deployer",
    "datadog-http-handler",
    "datadog-lambda-python",
    "datadog-linter",
    "datadog-log",
    "datadog-logger",
    "datadog-logs-python",
    "datadog-metrics",
    "datadog-monitor-deployer",
    "datadog-monitors-linter",
    "datadog-muted-alert-checker",
    "datadog-pandas",
    "datadog-serverless-compat",
    "datadog-serverless-utils",
    "datadog-sma",
    "datadog-threadstats",
]

def run_command(args, preexec_fn=None):
    """
    Execute a shell command and return its output, errors, and return code.

    Returns a (stdout, stderr, returncode) tuple.  A non-zero returncode means
    the command failed; callers must check it rather than assuming success.
    """
    print(f"Running command: '{' '.join(args)}'")
    try:
        result = subprocess.run(args, text=True, capture_output=True, check=True, preexec_fn=preexec_fn)
        return result.stdout, result.stderr, 0
    except subprocess.CalledProcessError as e:
        print(f"Command '{e.cmd}' failed with return code: {e.returncode}")
        print(f"Error: {e.stderr}")
        return e.stdout, e.stderr, e.returncode

def extract_version(req):
    """
    Extract version from the specifier string using packaging.
    """
    try:
        # Parse the specifier and get the first version from the specifier set
        version_spec = next(iter(req.specifier), None)
        return str(version_spec.version) if version_spec else None
    except Exception as e:
        print(f"Error parsing specifier: {e}")
        return None

def pre_python_installed_packages_file(directory):
    """
    Create pre installed packages file path.
    """
    return os.path.join(directory, '.pre_python_installed_packages.txt')

def post_python_installed_packages_file(directory):
    """
    Create post installed packages file path.
    """
    return os.path.join(directory, '.post_python_installed_packages.txt')

def diff_python_installed_packages_file(directory):
    """
    Create diff installed packages file path.
    """
    return os.path.join(directory, '.diff_python_installed_packages.txt')

def requirements_agent_release_file(directory):
    """
    Create requirements agent release file path.
    """
    return os.path.join(directory, 'requirements-agent-release.txt')

def check_file_owner_system_windows(filename):
    """
    Check if the file is owned by the SYSTEM or Administrators user on Windows.
    """
    # check if file exists
    if not os.path.exists(filename):
        return True

    # get NT System account SID
    system_sid = win32security.ConvertStringSidToSid("S-1-5-18")

    # get administator SID
    administrators_sid = win32security.ConvertStringSidToSid("S-1-5-32-544")

    # get owner of file
    sd = win32security.GetFileSecurity(filename, win32security.OWNER_SECURITY_INFORMATION)
    owner_sid = sd.GetSecurityDescriptorOwner()

    # print owner SID
    print(f"{filename}: SID: {win32security.ConvertSidToStringSid(owner_sid)}")

    return owner_sid == system_sid or owner_sid == administrators_sid

def check_all_files_owner_system_windows(directory):
    """
    Check if all files used by this feature are owned by SYSTEM or Administrators.
    This prevents issues with files created prior to first install by unauthorized users
    being used to install arbitrary packaged at install time.
    The MSI sets the datadirectory permissions before running this script so we
    don't have to worry about TOCTOU.
    """
    files = []
    files.append(directory)
    files.append(pre_python_installed_packages_file(directory))
    files.append(post_python_installed_packages_file(directory))
    files.append(diff_python_installed_packages_file(directory))

    for file in files:
        if not check_file_owner_system_windows(file):
            print(f"{file} is not owned by SYSTEM or Administrators, it may have come from an untrusted source, aborting installation.")
            return False
    return True

def has_expected_diff_file_permissions(filename):
    """
    Validate that the diff packages file is owned by root or the dd-agent user
    and is not writable by group/others. Files written by older Agent versions
    are owned by dd-agent, so it is accepted as a trusted owner. Packages from
    this file are installed as dd-agent, so any other owner may have come from
    an untrusted source.
    """
    file_stat = os.stat(filename)
    if (file_stat.st_mode & 0o777) & 0o022:
        print(f"{filename} is writable by group/others, aborting third-party package installation.")
        print(f"To recover: remove {filename} and reinstall your custom integrations manually after the upgrade.")
        return False
    trusted_uids = {0}
    try:
        trusted_uids.add(pwd.getpwnam('dd-agent').pw_uid)
    except KeyError:
        pass
    if file_stat.st_uid not in trusted_uids:
        print(f"{filename} is not owned by root or dd-agent, aborting third-party package installation.")
        print(f"To recover: remove {filename} and reinstall your custom integrations manually after the upgrade.")
        return False
    return True

def dd_agent_demote():
    """
    Return a preexec_fn that drops privileges to the dd-agent user, so package
    installation code restored from the diff file never runs as root.
    """
    pw = pwd.getpwnam('dd-agent')
    gid, uid = pw.pw_gid, pw.pw_uid

    def demote():
        os.setgroups([gid])
        os.setgid(gid)
        os.setuid(uid)

    return demote


def create_python_installed_packages_file(filename):
    """
    Create a file listing the currently installed Python dependencies.
    """
    print(f"Creating file: '{filename}'")
    with open(filename, 'w', encoding='utf-8') as f:
        f.write(DO_NOT_REMOVE_WARNING_HEADER)
        installed_packages = importlib.metadata.distributions()
        for dist in installed_packages:
            if dist.metadata['Name'] is None or dist.version is None:
                continue
            f.write(f"{dist.metadata['Name']}=={dist.version}\n")
    if not os.name == 'nt':
        os.chmod(filename, 0o600)
        if os.geteuid() == 0:
            os.chown(filename, 0, 0)

def create_diff_installed_packages_file(directory, old_file, new_file):
    """
    Create a file listing the new or upgraded Python dependencies.
    """
    print(f"Computing diff: baseline='{old_file}', current='{new_file}'")
    old_packages = load_requirements(old_file)
    new_packages = load_requirements(new_file)
    diff_file = diff_python_installed_packages_file(directory)
    print(f"Creating file: '{diff_file}'")
    diff_entries = []
    with open(diff_file, 'w', encoding='utf-8') as f:
        f.write(DO_NOT_REMOVE_WARNING_HEADER)
        for package_name, (_, new_req_value) in new_packages.items():
            old_req = old_packages.get(package_name)
            if old_req:
                _, old_req_value = old_req
                # Extract and compare versions
                old_version_str = extract_version(old_req_value)
                new_version_str = extract_version(new_req_value)
                if old_version_str and new_version_str:
                    if packaging.version.parse(new_version_str) > packaging.version.parse(old_version_str):
                        f.write(f"{new_req_value}\n")
                        diff_entries.append(f"{new_req_value} (upgraded from {old_version_str})")
            else:
                # Package is new in the new file; include it
                f.write(f"{new_req_value}\n")
                diff_entries.append(f"{new_req_value} (new)")
    if diff_entries:
        print(f"Diff contains {len(diff_entries)} package(s) to restore: {', '.join(diff_entries)}")
    else:
        print("Diff is empty: no packages to restore")
    if not os.name == 'nt':
        os.chmod(diff_file, 0o600)
        if os.geteuid() == 0:
            os.chown(diff_file, 0, 0)

def install_datadog_package(package, install_directory):
    """
    Install a Datadog integration via the datadog-agent integration command.

    Retries once on failure.  Raises IntegrationInstallError if both attempts fail.
    """
    if os.name == 'nt':
        agent_cmd = os.path.join(install_directory, 'bin', 'agent.exe')
        args = [agent_cmd, 'integration', 'install', '-t', package, '-r']
    else:
        # Invoked by absolute path rather than by name because this runs as root
        # during package scripts, where a PATH lookup could resolve to a shadowed
        # binary in a directory writable by a non-root user.
        agent_cmd = os.path.join(install_directory, 'bin', 'agent', 'agent')
        args = [agent_cmd, 'integration', 'install', '-t', package, '-r']

    for attempt in range(1, 3):
        print(f"Installing Datadog integration '{package}' (attempt {attempt}/2)")
        _, stderr, rc = run_command(args)
        if rc == 0:
            print(f"Successfully installed Datadog integration '{package}'")
            return
        print(f"Failed to install '{package}' on attempt {attempt}/2 (exit {rc})")
        if attempt < 2:
            time.sleep(1)
            print(f"Retrying '{package}'...")
    raise IntegrationInstallError(package, rc, stderr)

def install_dependency_package(pip, package, preexec_fn=None):
    """
    Install a Python dependency via pip.

    When preexec_fn is set, the dependency is installed in two phases: wheels
    are downloaded and built with privileges dropped to dd-agent first, then
    pip installs them as root. Installing a wheel only unpacks files and
    generates entry-point scripts, so no package code runs as root. This also
    keeps embedded/bin root-owned: it contains privileged helpers that
    systemd executes as root.

    Retries once on failure.  Raises IntegrationInstallError if both attempts fail.
    """
    print(f"Installing python dependency: '{package}'")
    command = pip.copy()
    wheelhouse = None
    if preexec_fn is not None:
        pw = pwd.getpwnam('dd-agent')
        wheelhouse = tempfile.mkdtemp(prefix='datadog-agent-wheels-')
        os.chown(wheelhouse, pw.pw_uid, pw.pw_gid)
        download_command = command + ['wheel', '--wheel-dir', wheelhouse, '--no-cache-dir', package]
        install_command = command + ['install', '--no-index', '--find-links', wheelhouse, package]
    else:
        install_command = command + ['install', package]

    try:
        for attempt in range(1, 3):
            print(f"Installing python dependency '{package}' (attempt {attempt}/2)")
            if wheelhouse is not None:
                _, stderr, rc = run_command(download_command, preexec_fn)
                if rc == 0:
                    _, stderr, rc = run_command(install_command)
            else:
                _, stderr, rc = run_command(install_command)
            if rc == 0:
                print(f"Successfully installed python dependency '{package}'")
                return
            print(f"Failed to install '{package}' on attempt {attempt}/2 (exit {rc})")
            if attempt < 2:
                time.sleep(1)
                print(f"Retrying '{package}'...")
        raise IntegrationInstallError(package, rc, stderr)
    finally:
        if wheelhouse is not None:
            shutil.rmtree(wheelhouse, ignore_errors=True)

def install_diff_packages_file(install_directory, filename, exclude_filename):
    """
    Install all Datadog integrations and python dependencies from a file.

    Every package is attempted regardless of earlier failures.  If any packages
    could not be installed after retries, raises IntegrationsRestoreError with
    the full list of failures so the caller can surface them.
    """
    preexec_fn = None
    if os.name == 'nt':
        python_path = os.path.join(install_directory, "embedded3", "python.exe")
        pip = [python_path, '-m', 'pip']
    else:
        pip = [os.path.join(install_directory, "embedded", "bin", "pip")]
        if os.geteuid() == 0:
            preexec_fn = dd_agent_demote()
    print(f"Installing python packages from: '{filename}'")
    install_packages = load_requirements(filename)
    exclude_packages = load_requirements(exclude_filename)
    attempted = 0
    skipped = 0
    failures = []
    for install_package_name, (install_package_line, _) in install_packages.items():
        if install_package_name in exclude_packages:
            skipped += 1
            print(f"Skipping '{install_package_name}' as it's already included in '{exclude_filename}' file")
        else:
            attempted += 1
            dep_name = packaging.requirements.Requirement(install_package_line).name
            try:
                if install_package_line.startswith('datadog-') and dep_name not in DEPS_STARTING_WITH_DATADOG:
                    install_datadog_package(install_package_line, install_directory)
                else:
                    install_dependency_package(pip, install_package_line, preexec_fn)
            except IntegrationInstallError as e:
                print(f"ERROR: {e}")
                failures.append(e)
    restored = attempted - len(failures)
    print(f"Restore summary: {restored}/{attempted} package(s) restored successfully ({skipped} skipped)")
    if failures:
        names = ", ".join(e.package for e in failures)
        print(f"ERROR: failed to restore {len(failures)} package(s): {names}")
        raise IntegrationsRestoreError(failures)

def load_requirements(filename):
    """
    Load requirements from a file.
    """
    print(f"Loading requirements from file: '{filename}'")
    valid_requirements = []
    with open(filename, 'r', encoding='utf-8') as f:
        raw_requirements = f.readlines()
        for req in raw_requirements:
            req_stripped = req.strip()
            # Skip and print reasons for skipping certain lines
            if not req_stripped:
                print(f"Skipping blank line: {req!r}")
            elif req_stripped.startswith('#'):
                print(f"Skipping comment: {req!r}")
            elif req_stripped.startswith(('-e', '--editable')):
                print(f"Skipping editable requirement: {req!r}")
            elif req_stripped.startswith(('-c', '--constraint')):
                print(f"Skipping constraint file reference: {req!r}")
            elif req_stripped.startswith(('-r', '--requirement')):
                print(f"Skipping requirement file reference: {req!r}")
            elif req_stripped.startswith(('http://', 'https://', 'git+', 'ftp://')):
                print(f"Skipping URL or VCS package: {req!r}")
            elif req_stripped.startswith('.'):
                print(f"Skipping local directory reference: {req!r}")
            elif req_stripped.endswith(('.whl', '.zip')):
                print(f"Skipping direct file reference (whl/zip): {req!r}")
            elif req_stripped.startswith('--'):
                print(f"Skipping pip flag: {req!r}")
            else:
                # Add valid requirement to the list
                valid_requirements.append(req_stripped)
    # Parse valid requirements using packaging
    requirements = {}
    for req_stripped in valid_requirements:
        req = packaging.requirements.Requirement(req_stripped)
        if req.url is not None:
            print(f"Skipping requirement with direct URL reference: {req_stripped!r}")
            continue
        requirements[req.name] = (req_stripped, req)
    return requirements

def cleanup_files(*files):
    """
    Remove the specified files.
    """
    for file in files:
        if os.path.exists(file):
            print(f"Removing file: '{file}'")
            os.remove(file)
