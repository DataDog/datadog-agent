"""
This module provides functions for managing Datadog integrations and Python dependencies after installation

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python postinst.py /path/to/install/dir`
"""

import os
import sys
import packages

def main():
    try:
        if len(sys.argv) != 2:
            print("Usage: postinst.py <INSTALL_DIR>")
        install_directory = sys.argv[1]
        if os.path.exists(install_directory):
            postinst_python_installed_packages_file = packages.postinst_python_installed_packages_file(install_directory)
            packages.create_python_installed_packages_file(postinst_python_installed_packages_file)
            flag_path = f"{install_directory}/.install_python_third_party_deps"
            if os.path.exists(flag_path):
                print(f"File '{flag_path}' found")
                diff_python_installed_packages_file = packages.diff_python_installed_packages_file(install_directory)
                if os.path.exists(diff_python_installed_packages_file):
                    requirements_agent_release_file = packages.requirements_agent_release_file(install_directory)
                    packages.install_diff_packages_file(f"{install_directory}/embedded/bin/pip", diff_python_installed_packages_file, requirements_agent_release_file)
                    packages.cleanup_files(diff_python_installed_packages_file)
                else:
                    print(f"File '{diff_python_installed_packages_file}' not found.")
            else:
                print(f"File '{flag_path}' not found: no third party integration will be installed.")
        else:
            print(f"Directory '{install_directory}' not found.")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    main()
