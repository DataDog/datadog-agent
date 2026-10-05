
rule yarahub_win_remcos_rat_unpacked_aug_2023
{
	meta:
		author = "Matthew @ Embee_Research"
		yarahub_author_twitter = "@embee_research"
		desc = "Detects bytecodes present in Amadey Bot Samples"
		sha_256 = "ec901217558e77f2f449031a6a1190b1e99b30fa1bb8d8dabc3a99bc69833784"
		date = "2023-08-27"
        yarahub_uuid = "f701cf05-ac09-44f3-b4ee-3ea944bd5533"
       	yarahub_license = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "57b00a449fc132c2f5d139c6d1cee7cd"
        malpedia_family = "win.remcos"
		
	strings:
		$r0 = " ______                              " ascii
		$r1 = "(_____ \\                             " ascii
		$r2 = " _____) )_____ ____   ____ ___   ___ " ascii 
		$r3 = "|  __  /| ___ |    \\ / ___) _ \\ /___)" ascii
		$r4 = "| |  \\ \\| ____| | | ( (__| |_| |___ |" ascii
		$r5 = "|_|   |_|_____)_|_|_|\\____)___/(___/ " ascii
		
		$s1 = "Watchdog module activated" ascii
		$s2 = "Remcos restarted by watchdog!" ascii
		$s3 = " BreakingSecurity.net" ascii

	condition:
		(
			(all of ($r*)) or (all of ($s*))
		)
}