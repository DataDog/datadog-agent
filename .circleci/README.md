# CircleCI

CircleCI is used to run unit tests on Unix env.

## Upgrading Golang version

/!\ Disclaimer: the datadog/agent-buildimages-circleci-runner image should never be used for anything else than CircleCI tests /!\

This image is now built alongside other images in [agent-buildimages](https://github.com/DataDog/datadog-agent-buildimages) repository. Change of Golang version must occur in this repository.

Once you have created a new image by building a new version of agent-buildimages, you can test your modification with the associated invoke task:

```bash
invoke -e pipeline.update-buildimages --image-tag v12345678-c0mm1t5
```
This will update the configuration of circleci and gitlab to use the __test version__ of these images.
Once your test is successful, you can either move the `_test_version` from files or invoke
```bash
invoke -e pipeline.update-buildimages --image-tag v12345678-c0mm1t5 --no-test-version
```

If everything is green, get a review and merge the PR.
