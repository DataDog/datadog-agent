rule BlackMoon {
    meta:
        author = "NDA0E"
	yarahub_author_twitter = "@NDA0E"
        date = "2024-10-20"
        description = "Detects BlackMoon"
	yarahub_uuid = "dc531539-588e-400b-8caa-a6e5af5ca6fc"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "0a554c494685e86c116fb212e5f38db7"
        malpedia_family = "win.krbanker"
    
    strings:
        $str0 = "blackmoon" ascii
        $str1 = "BlackMoon RunTime Error:" ascii
        
    condition:
	uint16(0) == 0x5a4d and 
        all of them
}