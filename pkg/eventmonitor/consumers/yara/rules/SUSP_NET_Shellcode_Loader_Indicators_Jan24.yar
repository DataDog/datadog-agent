rule SUSP_NET_Shellcode_Loader_Indicators_Jan24 {
    meta:
        description = "Detects indicators of shellcode loaders in .NET binaries"
        author = "Jonathan Peters"
        date = "2024-01-11"
        reference = "https://github.com/Workingdaturah/Payload-Generator/tree/main"
        hash = "c48752a5b07b58596564f13301276dd5b700bd648a04af2e27d3f78512a06408"
        score = 65
        yarahub_uuid = "eda4aae4-e33a-4a8c-9992-7979609bbde8"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "f03b6f7bff89bcba31d69706d3644350"
    strings:
        $sa1 = "VirtualProtect" ascii
        $sa2 = "VirtualAlloc" ascii
        $sa3 = "WriteProcessMemory" ascii
        $sa4 = "CreateRemoteThread" ascii
        $sa5 = "CreateThread" ascii
        $sa6 = "WaitForSingleObject" ascii
        $x = "__StaticArrayInitTypeSize=" ascii
    condition:
        uint16 ( 0 ) == 0x5a4d and 3 of ( $sa* ) and #x == 1
}