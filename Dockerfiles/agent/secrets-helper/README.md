# Helper script to access secret files (BETA)


**This feature is in beta and its options or behaviour might break between minor or bugfix releases of the Agent.**

Many of our integrations require credentials to retrieve metrics. To avoid hardcoding these credentials in the [Autodiscovery templates](https://docs.datadoghq.com/agent/autodiscovery/), you can use this feature to separate them from the template itself.

This script is available in the docker image as `/readsecret.py` and is intended
to be used with [the agent's external secret feature](https://github.com/DataDog/datadog-agent/blob/6.4.x/docs/agent/secrets.md). Please refer to this feature's documentation for usage examples.

## Script usage

- The script requires a folder passed as argument. Secret handles will be interpreted as file names, relative to this folder. The script will refuse to access any file out of this root folder (including symbolic link targets), in order to avoid leaking sensitive information.

- For now, this script is incompatible with [OpenShift restricted SCC operations](https://github.com/DataDog/datadog-agent/blob/6.4.x/Dockerfiles/agent/OPENSHIFT.md#restricted-scc-operations) and requires that the Agent runs as the `root` user.

- `ENV[]` tokens found in `datadog.yaml` and in Autodiscovery templates will be replaced, but replacing `ENV[]` tokens in config values passed as enviroment variables is not supported as of 6.5.0

## Setup examples

### Docker Swarm Secrets

[Docker secrets](https://docs.docker.com/engine/swarm/secrets/) are mounted in the `/run/secrets` folder. You need to pass the following environment variables to your agent container:

- `DD_SECRET_BACKEND_COMMAND=/readsecret.py`
- `DD_SECRET_BACKEND_ARGUMENTS=/run/secrets`

To use the `db_prod_password` secret value, exposed in the `/run/secrets/db_prod_password` file, just insert `ENC[db_prod_password]` in your template.

### Kubernetes secrets

Kubernetes supports [exposing secrets as files](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#create-a-pod-that-has-access-to-the-secret-data-through-a-volume) inside a pod.

If your secrets are mounted in `/etc/secret-volume`, just use the following environment variables:

- `DD_SECRET_BACKEND_COMMAND=/readsecret.py`
- `DD_SECRET_BACKEND_ARGUMENTS=/etc/secret-volume`

Following the linked example, the password field will be stored in the `/etc/secret-volume/password` file, and accessible via the `ENC[password]` token.

**Note:** We recommend using a dedicated folder instead of `/var/run/secrets`, as the script will be able to access all subfolders, including the sensitive `/var/run/secrets/kubernetes.io/serviceaccount/token` file.
