
Get-ChildItem 'entrypoint-ps1' | ForEach-Object {
	& $_.FullName
	if (-Not $?) {
		exit 1
	}
}

# Set process environment variables (from Docker) from Process to Machine level to allow Windows Services
# (process-agent, trace-agent) to get their configuration properly
foreach($key in [System.Environment]::GetEnvironmentVariables([System.EnvironmentVariableTarget]::Process).Keys) {
	if ($key.StartsWith("DD_") -Or $key -Like "*PROXY*") {
		Write-Output "Setting ENV var: $key to machine scope"
		$value = [System.Environment]::GetEnvironmentVariable($key, [System.EnvironmentVariableTarget]::Process)
		[System.Environment]::SetEnvironmentVariable($key, $value, [System.EnvironmentVariableTarget]::Machine)
	}
}

$agent = $args[0]
$agentArgs = $args | Select-Object -Skip 1
return & $agent $agentArgs
