## DataDog Agent 6 GUI
A cross-platform GUI for interacting with DataDog Agent 6.

#### Using the GUI
1. Configure a GUI port in your `datadog.yaml` file (i.e. `8080`)
  - Setting the port to -1 disables the GUI all together (this is the default state)
2. Restart the DataDog agent
3. Use the `datadog-agent launch-gui` command to launch the GUI within your default web browser

#### Requirements
1. Cookies must be enabled in your browser. The GUI generates and saves a token in your browser which is used for authenticating all communications with the GUI server.

2. The GUI will only be launched if the user launching it has the correct user permissions: if you are able to open `datadog.yaml`, you are able to use the GUI.

3. For security reasons, the GUI can **only** be accessed from the local network interface (```localhost```/```127.0.0.1```), so you must be on the same host that the agent is running to use it. In other words, you can't run the agent on a VM and access it from the host machine.

#### In development
- The 'Restart Agent' feature is not yet implemented for non-windows platforms
