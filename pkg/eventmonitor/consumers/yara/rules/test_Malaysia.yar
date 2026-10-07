rule test_Malaysia {
    meta:
        author = "rectifyq"
        yarahub_author_twitter = "@_rectifyq"
        date = "2024-09-06"
        description = "Detects file containing malaysia string"
        yarahub_uuid = "e33a3467-675f-48b0-b491-951d3b537b9b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "3F0E49C46CBDE0C7ADF5EA04A97AB261"
    
    strings:
        $malaysia = "malaysia" nocase
        $domain = "com.my" nocase
        
    condition:
        any of them
}  