import os
if not os.name == 'nt':
    import pwd
    import grp
else:
    import win32security
import importlib.metadata
import packaging
import subprocess

import packaging.requirements
import packaging.version

DO_NOT_REMOVE_WARNING_HEADER = "# DO NOT REMOVE/MODIFY - used internally by installation process\n"

def run_command(args):
    """
    Execute a shell command and return its output and errors.
    """
    try:
        print(f"Running command: '{' '.join(args)}'")
        result = subprocess.run(args, text=True, capture_output=True, check=True)
        return result.stdout, result.stderr
    except subprocess.CalledProcessError as e:
        print(f"Command '{e.cmd}' failed with return code: {e.returncode}")
        print(f"Error: {e.stderr}")
        return e.stdout, e.stderr

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


def create_python_installed_packages_file(filename):
    """
    Create a file listing the currently installed Python dependencies.
    """
    print(f"Creating file: '{filename}'")
    with open(filename, 'w', encoding='utf-8') as f:
        f.write(DO_NOT_REMOVE_WARNING_HEADER)
        installed_packages = importlib.metadata.distributions()
        for dist in installed_packages:
            f.write(f"{dist.metadata['Name']}=={dist.version}\n")
    if not os.name == 'nt':
        os.chmod(filename, 0o644)
        os.chown(filename, pwd.getpwnam('dd-agent').pw_uid, grp.getgrnam('dd-agent').gr_gid)

def create_diff_installed_packages_file(directory, old_file, new_file):
    """
    Create a file listing the new or upgraded Python dependencies.
    """
    old_packages = load_requirements(old_file)
    new_packages = load_requirements(new_file)
    diff_file = diff_python_installed_packages_file(directory)
    print(f"Creating file: '{diff_file}'")
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
            else:
                # Package is new in the new file; include it
                f.write(f"{new_req_value}\n")
    if not os.name == 'nt':
        os.chmod(diff_file, 0o644)
        os.chown(diff_file, pwd.getpwnam('dd-agent').pw_uid, grp.getgrnam('dd-agent').gr_gid)

def install_datadog_package(package, install_directory):
    """
    Install Datadog integrations running datadog-agent command
    """
    if os.name == 'nt':
        agent_cmd = os.path.join(install_directory, 'bin', 'agent.exe')
        args = [agent_cmd, 'integration', 'install', '-t', package, '-r']
    else:
        args = ['datadog-agent', 'integration', 'install', '-t', package, '-r']

    run_command(args)

def install_dependency_package(pip, package):
    """
    Install python dependency running pip install command
    """
    print(f"Installing python dependency: '{package}'")
    command = pip.copy()
    command.extend(['install', package])
    run_command(command)

def install_diff_packages_file(install_directory, filename, exclude_filename):
    """
    Install all Datadog integrations and python dependencies from a file
    """
    if os.name == 'nt':
        python_path = os.path.join(install_directory, "embedded3", "python.exe")
        pip = [python_path, '-m', 'pip']
    else:
        pip = [os.path.join(install_directory, "embedded", "bin", "pip")]
    print(f"Installing python packages from: '{filename}'")
    install_packages = load_requirements(filename)
    exclude_packages = load_requirements(exclude_filename)
    for install_package_name, (install_package_line, _) in install_packages.items():
        if install_package_name in exclude_packages:
            print(f"Skipping '{install_package_name}' as it's already included in '{exclude_filename}' file")
        else:
            if install_package_line.startswith('datadog-'):
                install_datadog_package(install_package_line, install_directory)
            else:
                install_dependency_package(pip, install_package_line)

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
    return {
        req.name: (req_stripped, req)
        for req_stripped, req in zip(valid_requirements, (packaging.requirements.Requirement(r) for r in valid_requirements))
    }

def cleanup_files(*files):
    """
    Remove the specified files.
    """
    for file in files:
        if os.path.exists(file):
            print(f"Removing file: '{file}'")
            os.remove(file)
