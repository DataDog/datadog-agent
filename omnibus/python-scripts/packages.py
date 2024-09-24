import os
import pwd
import grp
import importlib.metadata
import pkg_resources
from packaging import version
import subprocess

DO_NOT_REMOVE_WARNING_HEADER = "# DO NOT REMOVE/MODIFY - used internally by installation process\n"

def run_command(command):
    """
    Execute a shell command and return its output.
    """
    try:
        print(f"Running command: '{command}'")
        subprocess.run(command, shell=True, text=True, capture_output=True, check=True)
    except subprocess.CalledProcessError as e:
        print(f"Command '{e.cmd}' failed with return code: {e.returncode}")
        print(f"Error: {e.stderr}")

def extract_version(specifier):
    """
    Extract version from the specifier string.
    """
    try:
        # Get the first version specifier from the specifier string
        return str(next(iter(pkg_resources.Requirement.parse(f'{specifier}').specifier)))
    except Exception:
        return None

def prerm_python_installed_packages_file(directory):
    """
    Create prerm installed packages file path.
    """
    return os.path.join(directory, '.prerm_python_installed_packages.txt')

def postinst_python_installed_packages_file(directory):
    """
    Create postinst installed packages file path.
    """
    return os.path.join(directory, '.postinst_python_installed_packages.txt')

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
                old_version_str = extract_version(str(old_req_value.specifier))
                new_version_str = extract_version(str(new_req_value.specifier))
                if old_version_str and new_version_str:
                    if version.parse(new_version_str) > version.parse(old_version_str):
                        f.write(f"{new_req_value}\n")
            else:
                # Package is new in the new file; include it
                f.write(f"{new_req_value}\n")

def install_datadog_package(package):
    """
    Install Datadog integrations running datadog-agent command
    """
    print(f"Installing datadog integration: '{package}'")
    run_command(f'datadog-agent integration install -t {package} -r')

def install_dependency_package(pip, package):
    """
    Install python dependency running pip install command
    """
    print(f"Installing python dependency: '{package}'")
    run_command(f'{pip} install {package}')

def install_diff_packages_file(pip, filename, exclude_filename):
    """
    Install all Datadog integrations and python dependencies from a file
    """
    print(f"Installing python packages from: '{filename}'")
    install_packages = load_requirements(filename)
    exclude_packages = load_requirements(exclude_filename)
    for install_package_name, (install_package_line, _) in install_packages.items():
        if install_package_name in exclude_packages:
            print(f"Skipping '{install_package_name}' as it's already included in '{exclude_filename}' file")
        else:
            if install_package_line.startswith('datadog-'):
                install_datadog_package(install_package_line)
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
    return {req.name: (req_stripped, req) for req_stripped, req in zip(valid_requirements, pkg_resources.parse_requirements(valid_requirements))}

def cleanup_files(*files):
    """
    Remove the specified files.
    """
    for file in files:
        if os.path.exists(file):
            print(f"Removing file: '{file}'")
            os.remove(file)
