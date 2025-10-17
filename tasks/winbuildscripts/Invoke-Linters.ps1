& pip install git+https://github.com/DataDog/datadog-agent-dev.git@kfairise/support-feature-flag-ci
Write-Host "Invoking dda"
$Env:DDA_VERBOSE = "1"
& dda info owners code .gitlab-ci.yaml
Write-Host "dda info owners code .gitlab-ci.yaml result is $LASTEXITCODE"

& dda bzl
