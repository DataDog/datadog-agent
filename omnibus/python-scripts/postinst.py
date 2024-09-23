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
            env_var = 'INSTALL_PYTHON_THIRD_PARTY_DEPS'
            if env_var in os.environ:
                env_var_value = os.environ[env_var].lower() in ['true', '1', 't', 'yes', 'y']
                if env_var_value:
                    diff_python_installed_packages_file = packages.diff_python_installed_packages_file(install_directory)
                    if os.path.exists(diff_python_installed_packages_file):
                        requirements_agent_release_file = packages.requirements_agent_release_file(install_directory)
                        packages.install_diff_packages_file(f"{install_directory}/embedded/bin/pip", diff_python_installed_packages_file, requirements_agent_release_file)
                        packages.cleanup_files(diff_python_installed_packages_file)
                    else:
                        print(f"File {diff_python_installed_packages_file} does not exist.")
                else:
                    print(f"{env_var} is set to: {env_var_value}")
            else:
                print(f"{env_var} is not set.")
        else:
            print(f"Directory {install_directory} does not exist.")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    main()
