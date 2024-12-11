
function ExitWithCode($exitcode) {
	$host.SetShouldExit($exitcode)
	exit $exitcode
}
$result = install-windowsfeature -name Web-Server -IncludeManagementTools
if (! $result.Success ) {
	exit -1
}

$restartNeeded = $result.RestartNeeded

## adds addtional powershell commandlets
$result = install-windowsfeature web-scripting-tools

if (! $result.Success ) {
	exit -1
}

if ($restartNeeded -eq "Yes" -or $result.RestartNeeded -eq "Yes") {
	ExitWithCode(3010)
}
