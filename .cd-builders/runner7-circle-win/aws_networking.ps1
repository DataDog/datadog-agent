$ErrorActionPreference = 'Stop'

$defgw = Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Select-Object -ExpandProperty "NextHop"
Set-NetRoute -DestinationPrefix "169.254.169.254/32" -NextHop $defgw -ErrorAction 'silentlycontinue'
if(! $?){
    $defif = Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Select-Object -ExpandProperty "InterfaceIndex"
    New-NetRoute -DestinationPrefix "169.254.169.254/32" -NextHop $defgw -InterfaceIndex $defif
}
