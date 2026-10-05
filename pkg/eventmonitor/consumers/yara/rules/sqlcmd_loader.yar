rule sqlcmd_loader {
    meta:
        author = "@luc4m"
        date = "2023-03-26"
        hash_md5 = "6ffbbca108cfe838ca7138e381df210d"
        link = "https://medium.com/@lcam/updates-from-the-maas-new-threats-delivered-through-nullmixer-d45defc260d1"
        tlp = "WHITE"
	yarahub_uuid = "06196d3f-f414-4d87-9fe4-5dd40682f89f"
        yarahub_license =  "CC0 1.0"
        yarahub_rule_matching_tlp =  "TLP:WHITE"
        yarahub_rule_sharing_tlp =  "TLP:WHITE"
        yarahub_reference_md5= "6ffbbca108cfe838ca7138e381df210d" 
    strings:
        $trait_0 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 ec 04 00 00}
        $trait_1 = {85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 9f 04 00 00}
        $trait_2 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 7d 04 00 00}
        $trait_3 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 5b 04 00 00}
        $trait_4 = {6a 20 59 2b d9 03 f1 03 d1 3b d9 0f 83 5f fb ff ff}
        $trait_5 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 e3 03 00 00}
        $trait_6 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 c1 03 00 00}
        $trait_7 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 9f 03 00 00}
        $trait_8 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 4c 03 00 00}
        $trait_9 = {33 c9 85 ff 0f 9f c1 8d 0c 4d ?? ?? ?? ?? 85 c9 0f 85 2a 03 00 00}

 $str_0 = /debug[0-9]{1,3}\.ps1/i wide
 $str_1 = "%s\\\\sysnative\\\\%s" wide
 $str_2 = "/c \\\"powershell " wide
 $str_3 = "%s/ab%d.exe" wide 
 $str_4 = "%s/ab%d.php" wide 

    condition:
        (5 of ($trait_*)) and (3 of ($str_*))
}

