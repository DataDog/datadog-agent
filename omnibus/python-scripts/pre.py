"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python pre.py /path/to/install/dir`
"""

import os
import sys
import packages

# Older Agent versions wrote the integration baseline (.post_python_installed_packages.txt)
# to /opt/datadog-packages/tmp, which the installer reaps after 24h. The baseline now lives
# in the persistent run directory, but a baseline written by a previously-installed version
# may still be here, so we fall back to it. Linux/OCI only; on other platforms the path does
# not exist and is skipped.
LEGACY_OCI_STORAGE_LOCATION = "/opt/datadog-packages/tmp"

def find_baseline_file(storage_location, install_directory):
    """
    Locate the .post baseline, trying in order:
      1. the primary storage location (run dir for OCI, install dir for deb/rpm)
      2. the legacy OCI tmp directory (baselines written by older versions)
      3. the install directory (deb/rpm -> OCI migration)
    Returns the path to an existing baseline file, or None if none is found.
    """
    candidates = [storage_location]
    if os.name != 'nt':
        candidates.append(LEGACY_OCI_STORAGE_LOCATION)
    candidates.append(install_directory)
    seen = set()
    for directory in candidates:
        if not directory or directory in seen:
            continue
        seen.add(directory)
        candidate = packages.post_python_installed_packages_file(directory)
        if os.path.exists(candidate):
            print(f"Using baseline from: '{candidate}'")
            return candidate
        print(f"Baseline not found at: '{candidate}'")
    return None

def pre(install_directory, storage_location):
    print(f"pre: install_directory='{install_directory}', storage_location='{storage_location}'")
    try:
        if os.path.exists(install_directory) and os.path.exists(storage_location):
            post_python_installed_packages_file = find_baseline_file(storage_location, install_directory)
            if post_python_installed_packages_file:
                pre_python_installed_packages_file = packages.pre_python_installed_packages_file(storage_location)
                packages.create_python_installed_packages_file(pre_python_installed_packages_file)
                packages.create_diff_installed_packages_file(storage_location, post_python_installed_packages_file, pre_python_installed_packages_file)
                packages.cleanup_files(post_python_installed_packages_file, pre_python_installed_packages_file)
            else:
                # No baseline anywhere: the reaper may have removed it before the persistent
                # storage move, or this is a fresh install with no prior baseline. There is
                # nothing to diff, so no custom integrations need to be restored. Treat this
                # as a non-fatal no-op rather than an error to avoid failing the upgrade.
                print("No baseline file found in primary, legacy, or install locations; nothing to diff. Skipping custom integration save (non-fatal).")
                return 0
        else:
            print(f"Directory {install_directory} and {storage_location} do not exist.")
            return 1
    except Exception as e:
        print(f"Error: {e}")
        return 1
    print("pre: completed successfully")
    return 0

if os.name == 'nt':
    def main():
        if len(sys.argv) != 3:
            print("Usage: pre.py <INSTALL_DIR> <WINDOWS_DATADOG_DATA_DIR>")
            return 1
        install_directory = sys.argv[1]
        data_dog_data_dir = sys.argv[2]
        # Check data dog data directory exists and files are owned by system
        # should be run here to prevent security issues
        if not os.path.exists(data_dog_data_dir):
            print(f"Directory {data_dog_data_dir} does not exist.")
            return 1
        if not packages.check_all_files_owner_system_windows(data_dog_data_dir):
            print("Files are not owned by system.")
            return 1
        return pre(install_directory, data_dog_data_dir)
else:
    def main():
        if len(sys.argv) == 2:
            install_directory = sys.argv[1]
            return pre(install_directory, install_directory)
        elif len(sys.argv) == 3:
            install_directory = sys.argv[1]
            storage_location = sys.argv[2]
            return pre(install_directory, storage_location)
        print("Usage: pre.py <INSTALL_DIR> [STORAGE_LOCATION]")
        return 1

if __name__ == '__main__':
    sys.exit(main())
