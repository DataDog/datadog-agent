$ErrorActionPreference = "Stop"
# ECR Login
$AWS_ECR_PASSWORD = (aws ecr get-login-password --region us-east-1)
docker login --username AWS --password "${AWS_ECR_PASSWORD}" 486234852809.dkr.ecr.us-east-1.amazonaws.com
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
# DockerHub login
$tmpfile = [System.IO.Path]::GetTempFileName()
& "C:\mnt\tools\ci\fetch_secret.ps1" -parameterName "$Env:DOCKER_REGISTRY_LOGIN" -tempFile "$tmpfile"
If ($lastExitCode -ne "0") {
    Write-Host "Previous command returned $lastExitCode"
    exit "$lastExitCode"
}
$DOCKER_REGISTRY_LOGIN = $(cat "$tmpfile")
& "C:\mnt\tools\ci\fetch_secret.ps1" -parameterName "$Env:DOCKER_REGISTRY_PWD" -tempFile "$tmpfile"
If ($lastExitCode -ne "0") {
    Write-Host "Previous command returned $lastExitCode"
    exit "$lastExitCode"
}
$DOCKER_REGISTRY_PWD = $(cat "$tmpfile")
Remove-Item "$tmpfile"
docker login --username "${DOCKER_REGISTRY_LOGIN}" --password "${DOCKER_REGISTRY_PWD}" "docker.io"
If ($lastExitCode -ne "0") {
    throw "Previous command returned $lastExitCode"
}
