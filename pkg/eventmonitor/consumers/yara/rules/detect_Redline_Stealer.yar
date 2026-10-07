rule detect_Redline_Stealer {
     meta:
        date = "2023-06-06"
        author ="Varp0s"
        yarahub_reference_md5     = "554d25724c8f6f53af8721d0ef6b6f42"
        yarahub_uuid = "671d6f32-8236-46b5-80e3-057192936607"
        yarahub_license =  "CC0 1.0"
        yarahub_rule_matching_tlp =  "TLP:WHITE"
        yarahub_rule_sharing_tlp =  "TLP:WHITE"
        tlp = "WHITE"

    strings:

        $req0 = {72 75 6E 64 6C 6C 33 32 2E 65 78 65 20 25 73 61} 
        $req1 = {43 6F 6E 74 72 6F 6C 20 50 61 6E 65 6C 5C 44 65}
        $req2 = {77 65 78 74 72 61 63 74 2E 70 64 62 00} 
        $req3 = {49 58 50 25 30 33 64 2E 54 4D 50 00}
        $req4 = {54 4D 50 34 33 35 31 24 2E 54 4D 50 00}
        $req5 = {43 6F 6D 6D 61 6E 64 2E 63 6F 6D 20 2F 63 20 25} 
        $req6 = {55 50 44 46 49 4C 45 25 6C 75 00}


              
    condition:
        all of them
}