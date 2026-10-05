rule Asyncrat_V3{
    meta:
        description = "Detects Asyncrat"
        author = "Trezzza"
        date = "2026-09-27"
        yarahub_uuid = "2b4f3651-345a-49bc-a3e1-70ac7cc4401c"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "33a1ee920abf31eca7385ea3b36bd4e7"

    strings:
        $s1 = "\\nuR\\noisreVtnerruC\\swodniW\\tfosorciM\\erawtfoS" wide
        $s2 = "Select * from AntivirusProduct" wide
        $s3 = "/c schtasks /create /f /sc onlogon /rl highest /tn " wide

        $h1 = {bf eb 1e 56 fb cd 97 3b b2 19 02 24 30 a5 78 43 00 3d 56 44 d2 1e 62 b9 d4 f1 80 e7 e6 c3 39 41}
        
        $x1 = "188.212.158.203" ascii wide
        $x2 = "6606,7707,8808,1145" ascii wide
        $x3 = "9pnsEXQsA24S" ascii wide
    
    condition:
        uint16(0) == 0x5A4D and (
            (all of ($s*) and $h1) or 1 of ($x*)
        )
}