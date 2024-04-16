# Datadog Installer

This directory contains the `datadog-installer`, a tool developed by Datadog to facilitate the installation and upgrading of Datadog's software packages, including the Agent and Tracers.

## Helper Binary

The `helper` binary included with this installer is crucial for certain installation and upgrade processes. It is specifically designed to temporarily elevate privileges when necessary, such as for restarting services or moving files to protected directories.

### Security Features

To ensure the security of your systems:

- **Restricted Execution**: The execution of the `helper` binary is strictly limited to the `dd-installer` user. This control ensures that only authorized installation processes can utilize elevated privileges.
- **Limited Scope**: The `helper` binary's elevated privileges are restricted to predefined tasks essential for the installation and maintenance of Datadog software, reducing the risk of unauthorized actions.
- **Execution Controls**: Elevated operations are performed strictly within the context of the installation process, with detailed logging of actions taken during each session.

For additional information on our security practices contact our support team.
