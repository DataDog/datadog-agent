"""
This module provides functions for managing Datadog integrations and Python dependencies.

Functions:
- run_command(command): Executes a shell command and returns its output.
- create_integrations_file(directory): Creates a file listing currently installed Datadog integrations.
- create_dependencies_file(directory): Creates a file listing currently installed Python dependencies, excluding Datadog packages.
- install_integrations(directory): Installs Datadog integrations listed in a file.
- install_dependencies(directory): Installs Python dependencies listed in a file.

Usage:
- The script should be run with a single argument specifying the installation directory.
- Example: `python script.py /path/to/install/dir`
"""

import os
import subprocess
import shutil
import sys

def run_command(command):
    """
    Execute a shell command and return its output.

    Args:
        command (str): The shell command to execute.

    Returns:
        str: The standard output of the command.

    Raises:
        subprocess.CalledProcessError: If the command exits with a non-zero status.
    """
    result = subprocess.run(command, shell=True, text=True, capture_output=True, check=True)
    return result.stdout.strip()

def create_integrations_file(directory):
    """
    Create a file listing the currently installed Datadog integrations.

    This function runs the `datadog-agent integration freeze` command, sorts the output,
    and writes it to a file named '.datadog_requirements.txt' in the specified directory.
    It also changes the file ownership to 'dd-agent:dd-agent'.

    Args:
        directory (str): The directory where the integrations file will be created.
    """
    print("Creating integrations file")
    datadog_req_file = os.path.join(directory, '.datadog_requirements.txt')
    
    with open(datadog_req_file, 'w', encoding='utf-8') as f:
        output = run_command('datadog-agent integration freeze | sort')
        f.write(output)
    
    shutil.chown(datadog_req_file, user='dd-agent', group='dd-agent')

def create_dependencies_file(directory):
    """
    Create a file listing the currently installed Python dependencies, excluding Datadog packages.

    This function runs the `pip freeze` command, filters out Datadog packages, sorts the output,
    and writes it to a file named '.python_requirements.txt' in the specified directory.
    It also changes the file ownership to 'dd-agent:dd-agent'.

    Args:
        directory (str): The directory where the dependencies file will be created.
    """
    print("Creating dependencies file")
    python_req_file = os.path.join(directory, '.python_requirements.txt')
    
    with open(python_req_file, 'w', encoding='utf-8') as f:
        output = run_command(f'{directory}/embedded/bin/pip freeze | grep -v "^datadog-" | sort')
        f.write(output)
    
    shutil.chown(python_req_file, user='dd-agent', group='dd-agent')

def install_integrations(directory):
    """
    Install Datadog integrations listed in the '.installed_datadog_requirements.txt' file.

    This function reads the integrations from the specified file and installs each one using
    the `datadog-agent integration install` command. The file is deleted after processing.

    Args:
        directory (str): The directory where the installed integrations file is located.
    """
    installed_datadog_req_file = os.path.join(directory, '.installed_datadog_requirements.txt')
    
    if os.path.exists(installed_datadog_req_file):
        with open(installed_datadog_req_file, 'r', encoding='utf-8') as f:
            for line in f:
                print(f"Installing integration: {line.strip()}")
                run_command(f'datadog-agent integration install -t {line.strip()} -r')
        
        os.remove(installed_datadog_req_file)

def install_dependencies(directory):
    """
    Install Python dependencies listed in the '.installed_python_requirements.txt' file.

    This function reads the dependencies from the specified file and installs them using
    the `pip install` command. The file is deleted after processing.

    Args:
        directory (str): The directory where the installed dependencies file is located.
    """
    installed_python_req_file = os.path.join(directory, '.installed_python_requirements.txt')
    
    if os.path.exists(installed_python_req_file):
        print("Installing dependencies from requirements file")
        run_command(f'{directory}/embedded/bin/pip install -r {installed_python_req_file}')
        
        os.remove(installed_python_req_file)

if __name__ == '__main__':
    if len(sys.argv) != 2:
        print("Usage: script.py <INSTALL_DIR>")
        sys.exit(1)

    # Use a different variable name here to avoid conflict
    install_directory = sys.argv[1]
    
    if os.path.exists(install_directory):
        create_integrations_file(install_directory)
        create_dependencies_file(install_directory)
        install_integrations(install_directory)
        install_dependencies(install_directory)
    else:
        print(f"Directory {install_directory} does not exist.")
        sys.exit(1)
