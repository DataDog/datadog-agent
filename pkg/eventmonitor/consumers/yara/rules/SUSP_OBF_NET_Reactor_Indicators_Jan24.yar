rule SUSP_OBF_NET_Reactor_Indicators_Jan24 {
    meta:
        description = "Detects indicators of .NET Reactors managed obfuscation. Reactor is a commercial obfuscation solution, pirated versions are often abused by threat actors."
        author = "Jonathan Peters"
        date = "2024-01-09"
        reference = "https://www.eziriz.com/dotnet_reactor.htm"
        hash = "be842a9de19cfbf42ea5a94e3143d58390a1abd1e72ebfec5deeb8107dddf038"
        score = 65
        yarahub_uuid = "31e4c303-4733-4a83-97f7-79c92cff6b75"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c5f83d292e8f495832e3f8ddcbb89a10"
    strings:
        $ = "{ 33 7B 00 [9] 00 2D 00 [9] 00 2D 00 [9] 00 2D 00 [9] 00 7D 00 }"
        $ = "{ 3C 50 72 69 76 61 74 65 49 6D 70 6C 65 6D 65 6E 74 61 74 69 6F 6E 44 65 74 61 69 6C 73 3E 7B [8] 2D [4] 2D [4] 2D [4] 2D [12] 7D }"
        $ = "{ 3C 4D 6F 64 75 6C 65 3E 7B [8] 2D [4] 2D [4] 2D [4] 2D [12] 7D }"
    condition:
        uint16 ( 0 ) == 0x5a4d and 2 of them
}