param(
        [Parameter(Mandatory = $true)][string] $file,
        [Parameter(Mandatory = $true)][string] $targetDir
    )


[Reflection.Assembly]::LoadFrom("$($Env:WIX)\Microsoft.Deployment.WindowsInstaller.dll")

write-host "Extracting files from merge module: "$file

if(![IO.Directory]::Exists($targetDir)) { new-item -type directory -path $targetDir }

$cabFile = join-path $targetDir "temp.cab"
if([IO.File]::Exists($cabFile)) { remove-item $cabFile }

$db = new-object Microsoft.Deployment.WindowsInstaller.DataBase($file, [Microsoft.Deployment.WindowsInstaller.DataBaseOpenMode]::ReadOnly)
$view = $db.OpenView("SELECT `Name`,`Data` FROM _Streams WHERE `Name`= 'MergeModule.CABinet'")
$view.Execute()
$record = $view.Fetch()
$record.GetStream(2, $cabFile)
$view.Dispose()

& "$($Env:WINDIR)\system32\expand" -F:* $cabFile $targetDir

remove-item $cabFile

$extractedFiles = get-childitem $targetDir
$hashFiles = @{}
foreach($extracted in $extractedFiles)
{
    try
    {
        $longName = $db.ExecuteScalar("SELECT `FileName` FROM `File` WHERE `File`='{0}'", $extracted.Name) 
    }
    catch 
    {
        write-host "$($extracted.Name) is not in the MSM file"
    }

    if($longName)
    {
        $longName = $LongName.SubString($LongName.IndexOf("|") + 1)
        Write-host $longName

        #There are duplicates in the 
        if($hashFiles.Contains($longName))
        {
            write-host "Removing duplicate of $longName"
            remove-item $extracted.FullName
        }
        else
        {
            write-host "Rename $($extracted.Name) to $longName"
            $hashFiles[$longName] = $extracted
            $targetFilePath = join-path $targetDir $longName
            if([IO.File]::Exists($targetFilePath)) {remove-item $targetFilePath}
            rename-item $extracted.FullName -NewName $longName    
        }
    }
}
$db.Dispose()
