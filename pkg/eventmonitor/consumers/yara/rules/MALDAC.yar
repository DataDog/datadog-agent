rule MALDAC {
	meta:
		author = "Jonathan Beierle"
		description = "Detects samples designed to use WDAC to disable AV/EDR. False positives may occur."
		rule_category = "Technique"
		usage = "Hunting and Identification"
		yarahub_uuid = "73418134-e6aa-4a2c-bd53-6fab85bb6c76"
		yarahub_reference_md5 = "d924fbc8593427d9b7cc4bd7bd899718"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		date = "2025-11-18"
		date_created = "25 August 2025"
		reference = "https://github.com/logangoins/Krueger"
		reference = "https://beierle.win/2024-12-20-Weaponizing-WDAC-Killing-the-Dreams-of-EDR/"
		reference = "https://beierle.win/2025-08-28-A-Nightmare-on-EDR-Street-WDACs-Revenge/"
	strings:
		/* Strings and bytes used to identify an embedded WDAC policy */
		$wdac_signature = { (07 | 08) 00 00 00 0E }
		$wdac_section_break = { 0E 37 44 A2 C9 44 06 4C B5 51 F6 01 6E 56 30 76 }
		$wdac_deny = { FF FF FF FF FF FF FF FF 00 00 00 00 00 00 00 00 }
		
		/* Potentially targeted security products */
		$product_microsoft_1 = "Windows Defender" wide nocase
		$product_microsoft_2 = "MpCmdRun" wide nocase
		$product_microsoft_3 = "MsMpEng" wide nocase
		$product_microsoft_4 = "NisSrv" wide nocase
		$product_microsoft_5 = "MsSense" wide nocase
		$product_microsoft_6 = "MsDefenderCoreService" wide nocase
		$product_microsoft_7 = "SecurityHealthService" wide nocase
		$product_microsoft_8 = "SenseCncProxy" wide nocase
		$product_microsoft_9 = "Antimalware Service Executable" wide nocase
		$product_microsoft_10 = "Antimalware Core Service" wide nocase
		$product_microsoft_11 = "Windows Defender Advanced Threat Protection Service Executable" wide nocase
		$product_crowdstrike_1 = "CSAgent.sys" wide nocase
		$product_crowdstrike_2 = "CSFalconService.exe" wide nocase
		$product_crowdstrike_3 = "CrowdStrike Falcon Sensor" wide nocase
		$product_crowdstrike_4 = "drivers\\CrowdStrike\\" wide nocase
		$product_crowdstrike_5 = "Program Files\\CrowdStrike\\" wide nocase
		$product_elastic_1 = "Elastic Defend" wide nocase
		$product_elastic_2 = "Elastic-Agent" wide nocase
		$product_tanium_1 = "Tanium" wide nocase
		$product_avast_1 = "Avast" wide nocase
		$product_kingsoft_1 = "kingsoft" wide nocase
		
	condition:
		$wdac_signature
		and #wdac_section_break >= 3
		and #wdac_deny >= 2
		and 2 of ($product_*)
}