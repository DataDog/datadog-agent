rule APT_Bitter_Maldoc_Verify {
    
    meta:
        description = "Detects Bitter (T-APT-17) shellcode in oleObject (CVE-2018-0798)"
        author = "SECUINFRA Falcon Team (@SI_FalconTeam)"
        tlp = "WHITE"
        yarahub_uuid = "d3bcf5e4-4d6c-48d1-89b1-31fc130ec65a"
        yarahub_reference_md5 = "a1d9e1dccfbba118d52f95ec6cc7c943"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@SI_FalconTeam"
        reference = "https://www.secuinfra.com/en/techtalk/whatever-floats-your-boat-bitter-apt-continues-to-target-bangladesh"
        date = "2022-06-01"
        hash0 = "0c7158f9fc2093caf5ea1e34d8b8fffce0780ffd25191fac9c9b52c3208bc450"
        hash1 = "bd0d25194634b2c74188cfa3be6668590e564e6fe26a6fe3335f95cbc943ce1d"
        hash2 = "3992d5a725126952f61b27d43bd4e03afa5fa4a694dca7cf8bbf555448795cd6"

    strings:
        // This rule is meant to be used for verification of a Bitter Maldoc
        // rather than a hunting rule since the oleObject it is matching is
        // compressed in the doc zip
        
        $xor_string0 = "LoadLibraryA" xor
        $xor_string1 = "urlmon.dll" xor
        $xor_string2 = "Shell32.dll" xor
        $xor_string3 = "ShellExecuteA" xor
        $xor_string4 = "MoveFileA" xor    
        $xor_string5 = "CreateDirectoryA" xor
        $xor_string6 = "C:\\Windows\\explorer" xor
        $padding = {000001128341000001128341000001128342000001128342}
    
    condition:
        3 of ($xor_string*)
        and $padding
}