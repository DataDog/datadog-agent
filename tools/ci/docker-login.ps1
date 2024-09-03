$ErrorActionPreference = "Stop"
# ECR Login
$AWS_ECR_PASSWORD = (aws ecr get-login-password --region us-east-1)
docker login --username AWS --password "${AWS_ECR_PASSWORD}" 486234852809.dkr.ecr.us-east-1.amazonaws.com
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
# DockerHub login
$tmpfile = [System.IO.Path]::GetTempFileName()
& "C:\mnt\tools\ci\aws_ssm_get_wrapper.ps1" "$Env:DOCKER_REGISTRY_LOGIN" > $tmpfile
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
$DOCKER_REGISTRY_LOGIN = $(cat $tmpfile)
& "C:\mnt\tools\ci\aws_ssm_get_wrapper.ps1" "$Env:DOCKER_REGISTRY_PWD" > $tmpfile
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
$DOCKER_REGISTRY_PWD = $(cat $tmpfile)
docker login --username "${DOCKER_REGISTRY_LOGIN}" --password "${DOCKER_REGISTRY_PWD}" "docker.io"
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
