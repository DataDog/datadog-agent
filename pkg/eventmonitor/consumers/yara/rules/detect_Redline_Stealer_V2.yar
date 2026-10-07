rule detect_Redline_Stealer_V2 {
     meta:
        date = "2023-06-06"
        author ="Varp0s"
        yarahub_reference_md5     = "554d25724c8f6f53af8921d0ef6b6f42"
        yarahub_uuid = "e20669f7-da89-41f6-abeb-c3b5a770530e"
        yarahub_license =  "CC0 1.0"
        yarahub_rule_matching_tlp =  "TLP:WHITE"
        yarahub_rule_sharing_tlp =  "TLP:WHITE"
        tlp = "WHITE"
    strings:

        $req0 = {41 00 75 00 74 00 68 00 6F 00 72 00 69 00 7A} 
        $req1 = {6E 00 65 00 74 00 2E 00 74 00 63 00 70 00 3A 00}
        $req3 = {44 00 65 00 63 00 63 00 69 00 65 00 00 00}
        $req4 = {61 00 6D 00 6B 00 6D 00 6A 00 6A 00 6D 00 6D 00}
        $req5 = {31 00 36 00 33 00 2E 00 31 00 32 00 33 00 2E 00}
        $req6 = {59 00 61 00 6E 00 64 00 65 00 78 00 5C 00 59 00}
        $req7 = {31 00 2A 00 2E 00 31 00 6C 00 31 00 64 00 31 00}

              
    condition:
        3 of them 
}