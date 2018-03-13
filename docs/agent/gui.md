# Graphical User Interface

Agent 6 deprecated Agent5's Windows Agent Manager GUI, replacing it with a
browser-based, cross-platform one.

## Using the GUI

The port which the GUI runs on can be configured in your `datadog.yaml` file.
Setting the port to -1 disables the GUI all together. By default it is enabled
on port `5002` on Windows and Mac, and is disabled on Linux.

Once the Agent is running, use the `datadog-agent launch-gui` command to launch
the GUI within your default web browser.

In addition, on Windows operating systems, the GUI can be launched by executing 
the `ddtray` executable with the `-launch-gui` flag. The `ddtray` executable 
is what is run when starting via the shortcut in the Applications folder.

The `ddtray` executable provides the system tray functionality, including the
ability to launch the web-based configuration gui, and start, stop and restart
Datadog services.  The system tray can also initiate a `flare` directly.

## Requirements

1. Cookies must be enabled in your browser. The GUI generates and saves a token
in your browser which is used for authenticating all communications with the GUI
server.

2. The GUI will only be launched if the user launching it has the correct user
permissions: if you are able to open `datadog.yaml`, you are able to use the GUI.

3. For security reasons, the GUI can **only** be accessed from the local network
interface (```localhost```/```127.0.0.1```), so you must be on the same host that
the agent is running to use it. In other words, you can't run the agent on a VM
or a container and access it from the host machine.

## In development
- The 'Restart Agent' feature is not yet implemented for non-windows platforms
