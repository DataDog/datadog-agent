"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python pre.py /path/to/install/dir`
"""

import os
import sys
import packages

# Older OCI Agents wrote the baseline to the 24h-reaped tmp dir; fall back to it for
# upgrades from those versions. Linux/OCI only (path absent elsewhere).
LEGACY_OCI_STORAGE_LOCATION = "/opt/datadog-packages/tmp"

def find_baseline_file(storage_location, install_directory):
    """Locate the .post baseline: primary storage, then legacy OCI tmp, then install dir."""
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
                # No baseline. A prior install is marked by embedded/.installed_by_pkg.txt;
                # absent means a genuine first install (nothing to diff, non-fatal).
                installed_by_pkg_file = os.path.join(install_directory, "embedded", ".installed_by_pkg.txt")
                if not os.path.exists(installed_by_pkg_file):
                    print(f"No prior installation detected ('{installed_by_pkg_file}' missing); treating as first install. Nothing to diff (non-fatal).")
                    return 0
                # Prior install exists but its baseline is gone: fail so it surfaces in telemetry.
                print("Baseline file missing despite an existing installation; cannot save custom integrations.")
                return 1
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
