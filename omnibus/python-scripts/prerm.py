"""
This module handles the cleanup of Datadog integrations and Python dependencies during package removal.

Functions:
- run_command(command): Executes a shell command and returns its output.
- create_new_integrations_file(directory): Creates a temporary file listing currently installed Datadog integrations.
- create_new_dependencies_file(directory): Creates a temporary file listing currently installed Python dependencies.
- compare_and_update_files(old_file, new_file, output_file): Compares old and new files and writes differences to an output file.
- cleanup_files(*files): Removes the specified files.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python script_prerm.py /path/to/install/dir`
"""

import os
import sys
import packages

def cleanup_files(*files):
    """
    Remove the specified files.
    """
    for file in files:
        if os.path.exists(file):
            print(f"Removing file: {file}")
            os.remove(file)

if __name__ == '__main__':
    if len(sys.argv) != 2:
        print("Usage: prerm.py <INSTALL_DIR>")
        sys.exit(1)

    install_directory = sys.argv[1]
    if os.path.exists(install_directory):
        postinst_python_installed_packages_file = os.path.join(install_directory, '.postinst_python_installed_packages.txt')
        if os.path.exists(postinst_python_installed_packages_file):
            prerm_python_installed_packages_file = os.path.join(install_directory, '.prerm_python_installed_packages.txt')
            packages.create_python_installed_packages_file(prerm_python_installed_packages_file)

            diff_python_installed_packages_file = os.path.join(install_directory, '.diff_python_installed_packages.txt')
            packages.create_diff_installed_packages_file(postinst_python_installed_packages_file, prerm_python_installed_packages_file, diff_python_installed_packages_file)

            cleanup_files(postinst_python_installed_packages_file, prerm_python_installed_packages_file)
        else:
            print(f"File {postinst_python_installed_packages_file} does not exist.")
            sys.exit(1)
    else:
        print(f"Directory {install_directory} does not exist.")
        sys.exit(1)
