param (
    [string]$parameterName
)

$retryCount = 0
$maxRetries = 10

while ($retryCount -lt $maxRetries) {
    $result = (aws ssm get-parameter --region us-east-1 --name $parameterName --with-decryption --query "Parameter.Value" --output text)

    if ($result) {
        $result
        break
    }

    $retryCount++
    Start-Sleep -Seconds ([math]::Pow(2, $retryCount))
}
