
function ExitWithCode($exitcode) {
	$host.SetShouldExit($exitcode)
	exit $exitcode
}
$result = install-windowsfeature -name Web-Server -IncludeManagementTools
if (! $result.Success ) {
	exit -1
}
if ($result.RestartNeeded -eq "Yes") {
	ExitWithCode(3010)
}
