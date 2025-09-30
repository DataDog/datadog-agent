"""
This module provides functions for managing Datadog integrations and Python dependencies after installation

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python post.py /path/to/install/dir`
"""

import os
import sys
import packages

def post(install_directory, storage_location):
    try:
        if os.path.exists(install_directory) and os.path.exists(storage_location):
            post_python_installed_packages_file = packages.post_python_installed_packages_file(storage_location)
            packages.create_python_installed_packages_file(post_python_installed_packages_file)

            flag_path = "/etc/datadog-agent/.skip_install_python_third_party_deps"
            if os.name == "nt":
                flag_path = os.path.join(storage_location, ".skip_install_python_third_party_deps")

            if not os.path.exists(flag_path):
                diff_python_installed_packages_file = packages.diff_python_installed_packages_file(storage_location)
                if os.path.exists(diff_python_installed_packages_file):
                    requirements_agent_release_file = packages.requirements_agent_release_file(install_directory)
                    # don't delete the diff file. This handles install failure cases on windows
                    # on uninstall/install if install fails we need the diff file to retry the install
                    packages.install_diff_packages_file(install_directory, diff_python_installed_packages_file, requirements_agent_release_file)
                else:
                    print(f"File '{diff_python_installed_packages_file}' not found.")
                    return 0
            else:
                if not os.path.exists(flag_path):
                    print(f"File '{flag_path}' found: no third party integration will be installed.")
                else:
                    print(f"No third party integration will be installed.")
                return 0
        else:
            print(f"Directory '{install_directory}' and '{storage_location}' not found.")
            return 1
    except Exception as e:
        print(f"Error: {e}")
        return 1
    return 0

if os.name == 'nt':
    def main():
        if len(sys.argv) != 3:
            print("Usage: post.py <INSTALL_DIR> <WINDOWS_DATADOG_DATA_DIR>")
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
        # The MSI uses its own flag to control whether or not this script is executed
        # so we skip/ignore the file-based flag used by other platforms.
        return post(install_directory, data_dog_data_dir)
else:
    def main():
        if len(sys.argv) == 2:
            install_directory = sys.argv[1]
            return post(install_directory, install_directory)
        elif len(sys.argv) == 3:
            install_directory = sys.argv[1]
            storage_location = sys.argv[2]
            return post(install_directory, storage_location)
        print("Usage: post.py <INSTALL_DIR> [STORAGE_LOCATION]")
        return 1

if __name__ == '__main__':
    sys.exit(main())
