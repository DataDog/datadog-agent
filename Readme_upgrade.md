# Keeping agent synchronized to original upstream

In order to merge tag marked as "lastest" on a original upstream

```shell

git checkout -b upstream-x-y-z
git pull --tags https://github.com/DataDog/datadog-agent.git x.y.z
... (resolve conflicts, if any)
git commit
git push
git push origin x.y.z

```
