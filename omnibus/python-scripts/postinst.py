"""
This module provides functions for managing Datadog integrations and Python dependencies after installation

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python postinst.py /path/to/install/dir`
"""

import os
import sys
import packages


if __name__ == '__main__':
    if len(sys.argv) != 2:
        print("Usage: postinst.py <INSTALL_DIR>")
        sys.exit(1)
    install_directory = sys.argv[1]
    if os.path.exists(install_directory):
        postinst_python_installed_packages_file = packages.postinst_python_installed_packages_file(install_directory)
        packages.create_python_installed_packages_file(postinst_python_installed_packages_file)
    else:
        print(f"Directory {install_directory} does not exist.")
        sys.exit(1)
