rule RABBITHUNT_loader {
  meta:
    date = "2022-06-13"
    author = "Willi Ballenthin"
    yarahub_author_email = "william.ballenthin@mandiant.com"
    yarahub_author_twitter = "@williballenthin"
    yarahub_uuid = "a0476975-9fb5-410e-90be-1a4acd6398e3"
    yarahub_license = "CC BY 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "22a968beda8a033eb31ae175b7e0a937"
  strings:
        $a = "kernel32.dll:LoadLibraryA"
        $b = "kernel32.dll:VirtualFree"
        $c = "kernel32.dll:VirtualAlloc"
        $d = "kernel32.dll:UnmapViewOfFile"
        $e = "kernel32.dll:GetFileAttributesW"
        $f = "kernel32.dll:GetFileSize"
        $g = "kernel32.dll:MapViewOfFile"
        $h = "kernel32.dll:CloseHandle"
        $i = "kernel32.dll:CreateFileW"
        $j = "kernel32.dll:CreateFileMappingW"
        
  condition:
    any of them
}