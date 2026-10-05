rule private_string_search {
    meta:
        date = "2025-12-08"
        yarahub_reference_md5 = "c4b6d8ffc103f65cbb533ad8aa659bcb"
        yarahub_uuid = "2a5b8100-7c23-4d44-8742-991df9960241"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE" 
        yarahub_rule_sharing_tlp = "TLP:WHITE" 
        author = "Researcher_Name"
        description = "Hunting for specific text strings"

    strings:
        $s1 = "x-apikey" ascii wide nocase
        $s2 = "virustotal.com" ascii wide nocase

    condition:
        $s1 and $s2
}