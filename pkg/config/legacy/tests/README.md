# Importer tests

`config.py` is imported from the Agent V5 where it's used to load datadog.conf.

The go test directly use `config.yaml` to validate the legacy importer.

## Refreshing the config.yaml

simply run (use of `jq` is optional):

```
python config.py | jq . > config.yaml
```
