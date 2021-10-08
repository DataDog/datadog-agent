## win-installopts

### Objective

Test the Windows command line installer options for tags, hostname, command port, all proxy settings, and datadog site-specific settings

### Installation Recipe

Uses the \_install_windows_base recipe to do a fresh install of the agent.
1. Uses a fresh install because install options are ignored on upgrades
2. Uses the \_base recipe which allows setting of additional options

### Test definition

Test is successful if the options are correctly written to `datadog.yaml`.  The verifier reads and parses the resulting configuration file to verify the test options are correctly written. 