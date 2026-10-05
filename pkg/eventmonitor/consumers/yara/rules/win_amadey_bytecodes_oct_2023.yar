
rule win_amadey_bytecodes_oct_2023
{
	meta:	
		author = "Matthew @ Embee_Research"
		yarahub_author_twitter = "@embee_research"
		desc = "Detects bytecodes present in Amadey Bot Samples"
		sha_256 = "4165190e60ad5abd437c7768174b12748d391b8b97c874b5bdf8d025c5e17f43"
		date = "2023-10-15"
        yarahub_uuid = "19e955f9-d125-41af-981b-09957a8abbc8"
       	yarahub_license = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "2ba1411c46d529f2ae6a7c154d13f029"
        malpedia_family = "win.amadey"

		
	strings:
		$s1 = {8b ?? fc 83 c1 23 2b c2 83 c0 fc 83 f8 1f 77}
		$s2 = {80 ?? ?? ?? 3d 75 }
		$s3 = {8b c1 c1 f8 10 88 ?? ?? 8b c1 c1 f8 08}
		
	condition:
		
		$s1 and $s2 and $s3
		

}