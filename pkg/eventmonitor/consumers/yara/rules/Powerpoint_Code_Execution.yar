rule Powerpoint_Code_Execution {

	meta:

		author = "Ahmet Payaslioglu"
		yarahub_author_twitter = "@Computeus7"
		date = "2022-09-15"
		description ="New code execution technique using Powerpoint has been seen in the wild. The technique is triggered by using hyperlinks instead of Run Program/Macro. This new method has bypassed all the vendors for 220 days since 2022-02-02."
		yarahub_reference_md5 = "c0060c0741833af67121390922c44f91"
		yarahub_reference_link = "https://www.linkedin.com/feed/update/urn:li:activity:6976093476027314176/" 
		yarahub_uuid = "9582d920-9bc4-4db3-9048-54ea56567dbd"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"


	strings:

		$a1 = {D0 CF 11 E0 A1 B1 1A E1} //header

		$b1 = {6C 00 6F 00 63 00 61 00 6C 00 2E 00 6C 00 6E 00 6B} //local.lnk

		$b2 = {6C 00 6D 00 61 00 70 00 69 00 32 00 2E 00 64 00 6C 00 6C 00} //lmapi2.dll

		$b3 = {72 00 75 00 6E 00 64 00 6C 00 6C 00 33 00 32} //rundll32.exe

		$b4 = {4E 00 65 00 74 00 2E 00 57 00 65 00 62 00 43 00 6C 00 69 00 65 00 6E 00 74 00 29 00 2E 00 44 00 6F 00 77 00 6E 00 6C 00 6F 00 61 00 64 00 44 00 61 00 74 00 61} //Net Web Client) Download Data

	condition:
		($a1 at 0) and (4 of ($b*)) and filesize < 2MB
}