rule ItsSoEasy_Ransomware_basic {
    meta:
        description = "Detect basics of ItsSoEasy Ransomware (Itssoeasy-A)"
        author = "bstnbuck"
        date = "2023-11-02"
        yarahub_author_twitter = "@bstnbuck"
        yarahub_reference_link = "https://github.com/bstnbuck/ItsSoEasy"
        yarahub_uuid = "a2564e9f-e5f9-459c-ae4b-7656fa9df9c3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1ce280542553dc383b768b9189808e27"
        
    strings:
        $typ1 = "itssoeasy" nocase
        $typ1_wide = "itssoeasy" nocase wide
        $typ2 = "itssoeasy" base64
        $typ3 = "ItsSoEasy" base64
	
    condition:
        any of them
}