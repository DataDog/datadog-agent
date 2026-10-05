rule LokiPWS {
    meta:
        author = "NDA0E"
	yarahub_author_twitter = "@NDA0E"
	date = "2024-10-20"
        description = "Detects LokiBot"
	yarahub_uuid = "d40652f1-a047-44ed-b00f-8e3321d7ed07"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6746fbb343ddec70416177f77ef83c2a"
        malpedia_family = "win.lokipws"
    
    strings:
        $str0 = "SELECT encryptedUsername, encryptedPassword, formSubmitURL, hostname FROM moz_logins" ascii
        $str1 = "%s%s\\Login Data" wide ascii
	$str2 = "sqlite3.dll" wide ascii		
        
    condition:
        uint16(0) == 0x5a4d and 
        all of them
}