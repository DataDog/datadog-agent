# Prerequisites

Check out and go through:

https://datadoghq.atlassian.net/wiki/spaces/AETET/pages/2964619265/Getting+started

# Running Locally

To invoke locally, run:

```bash
cd ~/dd/datadog-agent # run from the repo root, not the new-e2e project root
aws-vault exec sso-agent-sandbox-account-admin -- zsh
inv new-e2e-tests.run --targets=./tests/orchestrator
```

You can supply `--keep-stacks` to keep the pulumi stacks after the tests are done. This will allow you to use inspect
the test K8S cluster via kubectl/k9s.

You can supply `--extra-flags "--replace-stacks"` to destroy any existing infra before the test is setup.

## `kubectl`/`k9s`

You can update your `~/kube/config` to point to the test cluster:

```bash
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: https://10.1.58.202:8443
  name: kind-kind
- context:
    cluster: kind-kind
    user: kind-kind
  name: kind-kind
users:
- name: kind-kind
  user:
    client-certificate-data: <base64>
    client-key-data: <base64>
```

Replace the following values with the details printed out in the Pulumi stack output:
- `clusters[0].cluster.server`
- `users[0].user.client-certificate-data`
- `users[0].user.client-key-data`

Every time you replace/destroy your Pulumi stack, these details will change. You can use the generated command that is
printed out to update your kube config automatically. Look for the `COMMAND TO UPDATE LOCAL KUBECONFIG` log line. It
will look like this:

> Note: It will require that you `brew install yq` first.

```bash
cat ~/.kube/config \
  | yq '( .clusters[] | select(.name == "kind-kind") ).cluster.server |= "https://10.1.58.202:8443"' \
  | yq '( .users[] | select(.name == "kind-kind") ).user |= {"client-certificate-data": "<base64>", "client-key-data": "<base64>"}' \
  > ~/.kube/config_updated && mv ~/.kube/config_updated ~/.kube/config
```

## Automatic Cleanup

The AWS Sandbox account is cleaned up periodically. If it's been a few days since you've run the test locally, you may
run into errors like this when trying to run the tests:

```
Diagnostics:
  aws:acm:Certificate (aws-fakeintake-cert):
    error: 1 error occurred:
    	* updating urn:pulumi:fevans-kind-cluster::e2elocal::dd:fakeintake$aws:acm/certificate:Certificate::aws-fakeintake-cert: 1 error occurred:
    	* importing ACM Certificate (arn:aws:acm:us-east-1:376334461865:certificate/46ca0328-4d4e-463d-9aa3-da5b7f3bdf69): operation error ACM: ImportCertificate, https response error StatusCode: 400, RequestID: f6c45c07-9019-4a14-b950-401e73aa5006, ResourceNotFoundException: Could not find certificate arn:aws:acm:us-east-1:376334461865:certificate/46ca0328-4d4e-463d-9aa3-da5b7f3bdf69.

  pulumi:pulumi:Stack (e2elocal-fevans-kind-cluster):
    error: update failed

  command:remote:Command (remote-vm-connection-cmd-docker-whoami):
    error: after 60 failed attempts: dial tcp 10.1.61.167:22: i/o timeout
```

To fix this, run:

```bash
inv -e new-e2e-tests.clean -s
```

It may take a while, but will completely reset your pulumi stack config/state/resources.

# Custom Agent Version

You can specify your own agent version as well, otherwise it will run with latest.

> Note: in the CI builds, a specific version is automatically supplied to the test invocation.

```bash
img_repo="registry.hub.docker.com/datadog"
img_tag="fisher-cap-1436-explicit-type-values-py3-jmx"
inv new-e2e-tests.run \
  -c ddagent:fullImagePath=$img_repo/agent-dev:$img_tag \
  -c ddagent:clusterAgentFullImagePath=$img_repo/cluster-agent-dev:$img_tag \
  --targets=./tests/orchestrator
```

# Fake Intake

If you `--keep-stacks`, you can inspect the fake intake via curl.

```bash
fakehost="internal-fevans-kind-cluster-fakeintake-1279144000.us-east-1.elb.amazonaws.com"
curl -k -v "https://$fakehost/fakeintake/routestats" | jq
curl -k -v "https://$fakehost/fakeintake/payloads?endpoint=/api/v2/orchmanif" | jq
```

The `fakehost` is a pulumi stack output.
