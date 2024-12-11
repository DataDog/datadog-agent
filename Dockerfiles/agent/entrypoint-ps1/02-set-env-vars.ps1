# Set process environment variables (from Docker) from Process to Machine level to allow Windows Services
# (process-agent, trace-agent) to get their configuration properly
foreach($key in [System.Environment]::GetEnvironmentVariables([System.EnvironmentVariableTarget]::Process).Keys) {
	Write-Output "Setting ENV var: $key to machine scope"
	$value = [System.Environment]::GetEnvironmentVariable($key, [System.EnvironmentVariableTarget]::Process)
	[System.Environment]::SetEnvironmentVariable($key, $value, [System.EnvironmentVariableTarget]::Machine)
}
