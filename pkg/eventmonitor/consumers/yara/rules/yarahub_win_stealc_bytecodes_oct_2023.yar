
rule yarahub_win_stealc_bytecodes_oct_2023
{
	meta:
		author = "Matthew @ Embee_Research"
		yarahub_author_twitter = "@embee_research"
		desc = "Bytecodes present in Stealc decoding routine"
		sha_256 = "74ff68245745b9d4cec9ef3c539d8da15295bdc70caa6fdb0632acdd9be4130a"
		sha_256 = "9f44a4cbc30e7a05d7eb00b531a9b3a4ada5d49ecf585b48892643a189358526"
		date = "2023-10-13"
        yarahub_uuid = "614538a7-d5da-4d98-9fc3-6cf4d2f10fb4"
       	yarahub_license = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "768a03270a3ac83610a382bc18ee0021"
        malpedia_family = "win.stealc"
		
	strings:
		$s1 = {8b 4d f0 89 4d f8 8b 45 f8 c1 e0 03 33 d2 b9 06 00 00 00 f7 f1 8b e5 5d c2 04 00}
		
		
	condition:
		
		$s1

}