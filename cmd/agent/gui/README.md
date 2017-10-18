## DataDog Agent 6 GUI
A cross-platform GUI served on ```localhost:8080``` (by default) for interacting with DataDog Agent 6.

#### Requirements
1. Cookies must be enabled in your browser. Upon initially accessing the GUI, you will be asked once to enter your API key; after that, the key is saved as a cookie and is used for authenticating all communications with the GUI server.  

2. The GUI allows you to edit your configuration (yaml) files. Therefore it is necessary for you to ensure that the user running the DataDog Agent has the correct permissions for writing to these files - generally this means that the user must be the owner.

3. For security reasons, the GUI can **only** be accessed from the local network interface (```localhost```/```127.0.0.1```), so you must be on the same host that the agent is running to use it. In other words, you can't run the agent on a VM and access it from the host machine.

#### Disabling the GUI
The GUI can be disabled altogether by setting its port to -1 in your datadog.yaml file.

#### In development
- The 'Restart Agent' feature is not yet implemented for non-windows platforms
- A more robust authentication system is in progress
