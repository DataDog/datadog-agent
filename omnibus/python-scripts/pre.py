"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python pre.py /path/to/install/dir`
"""

import os
import sys
import packages

def pre(install_directory, storage_location):
    try:
        if os.path.exists(install_directory) and os.path.exists(storage_location):
            postinst_python_installed_packages_file = packages.postinst_python_installed_packages_file(storage_location)
            if os.path.exists(postinst_python_installed_packages_file):
                pre_python_installed_packages_file = packages.pre_python_installed_packages_file(storage_location)
                packages.create_python_installed_packages_file(pre_python_installed_packages_file)
                packages.create_diff_installed_packages_file(storage_location, postinst_python_installed_packages_file, pre_python_installed_packages_file)
                packages.cleanup_files(postinst_python_installed_packages_file, pre_python_installed_packages_file)
            else:
                print(f"File {postinst_python_installed_packages_file} does not exist.")
                return 1
        else:
            print(f"Directory {install_directory} and {storage_location} do not exist.")
            return 1
    except Exception as e:
        print(f"Error: {e}")
        return 1
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
        if len(sys.argv) != 2:
            print("Usage: pre.py <INSTALL_DIR>")
            return 1
        install_directory = sys.argv[1]
        return pre(install_directory, install_directory)



if __name__ == '__main__':
    sys.exit(main())
