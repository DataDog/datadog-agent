"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python prerm.py /path/to/install/dir`
"""

import os
import sys
import packages

def main():
    try:
        if len(sys.argv) != 2:
            print("Usage: prerm.py <INSTALL_DIR>")
        install_directory = sys.argv[1]
        if os.path.exists(install_directory):
            postinst_python_installed_packages_file = packages.postinst_python_installed_packages_file(install_directory)
            if os.path.exists(postinst_python_installed_packages_file):
                prerm_python_installed_packages_file = packages.prerm_python_installed_packages_file(install_directory)
                packages.create_python_installed_packages_file(prerm_python_installed_packages_file)
                packages.create_diff_installed_packages_file(install_directory, postinst_python_installed_packages_file, prerm_python_installed_packages_file)
                packages.cleanup_files(postinst_python_installed_packages_file, prerm_python_installed_packages_file)
            else:
                print(f"File {postinst_python_installed_packages_file} does not exist.")
        else:
            print(f"Directory {install_directory} does not exist.")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    main()
