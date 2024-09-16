param (
    [string]$parameterName,
    [string]$tempFile
)

$retryCount = 0
$maxRetries = 10

while ($retryCount -lt $maxRetries) {
    $result = (aws ssm get-parameter --region us-east-1 --name $parameterName --with-decryption --query "Parameter.Value" --output text 2> awsErrorFile.txt)
    $error = Get-Content awsErrorFile.txt
    if ($result) {
        "$result" | Out-File -FilePath "$tempFile" -Encoding ASCII
        exit 0
    }
    if ($error -match "Unable to locate credentials") {
        # See 5th row in https://docs.google.com/spreadsheets/d/1JvdN0N-RdNEeOJKmW_ByjBsr726E3ZocCKU8QoYchAc
        Write-Error "Permanent error: unable to locate AWS credentials, not retrying"
        exit 42
    }

    $retryCount++
    Start-Sleep -Seconds ([math]::Pow(2, $retryCount))
}

Write-Error "Failed to retrieve $parameterName after $maxRetries retries"
exit 1
