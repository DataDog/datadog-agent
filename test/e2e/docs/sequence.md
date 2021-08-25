# Generate sequence

## Update process

1. Copy paste the content of each sequence in the [online tool](https://github.com/mermaidjs/mermaid-live-editor).
2. Download the image generated
3. move it to replace the old one

### Online data

[setup-instance](../scripts/setup-instance):

```text
graph TD
A{setup-instance} -->B(AWS specification)
B --> C[ignition]
C --> D(sshAuthorizedKeys)
D -->B
B --> E[ec2]
E --> F(request-spot-instances)
F --> G(describe-spot-instance-requests)
G -->|Instance created| H(create-tags)
H -->|instance and spot requests| I(describe-instances)
I -->|Get PrivateIpAddress| J(cancel-spot-instance-requests)
J --> K[ssh]
K --> L(git clone and checkout)
L --> M{run-instance}
```


[run-instance](../scripts/run-instance)
```text
graph TD
A{Run Instance} -->B[kind create cluster]
B --> C[kind cluster ready]
C --> D[argo download]
D --> E[argo setup]
E --> F[argo submit]
F -->|wait completion| G[argo get results]
G --> H{exit with code}
```
