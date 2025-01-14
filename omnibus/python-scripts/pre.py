"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python pre.py /path/to/install/dir`
"""

import os
import sys
import packages

def main():
    try:
        if len(sys.argv) != 2:
            print("Usage: pre.py <INSTALL_DIR>")
        install_directory = sys.argv[1]
        if os.path.exists(install_directory):
            post_python_installed_packages_file = packages.post_python_installed_packages_file(install_directory)
            if os.path.exists(post_python_installed_packages_file):
                pre_python_installed_packages_file = packages.pre_python_installed_packages_file(install_directory)
                packages.create_python_installed_packages_file(pre_python_installed_packages_file)
                packages.create_diff_installed_packages_file(install_directory, post_python_installed_packages_file, pre_python_installed_packages_file)
                packages.cleanup_files(post_python_installed_packages_file, pre_python_installed_packages_file)
            else:
                print(f"File {post_python_installed_packages_file} does not exist.")
        else:
            print(f"Directory {install_directory} does not exist.")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    main()
