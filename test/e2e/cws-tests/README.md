# CSPM e2e tests

## Docker flavors

To run docker flavoured tests, local only, please run:

For CSPM:
```sh
DD_API_KEY=<API_KEY> \
DD_APP_KEY=<APP_KEY> \
DD_SITE=datadoghq.com \
DD_AGENT_IMAGE=datadog/agent-dev:master \
python3 tests/test_e2e_cspm_docker.py
```

Please change `DD_AGENT_IMAGE` to a branch specific tag if you need to test a specific branch.
