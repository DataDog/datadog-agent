rule ELF_RANSOMWARE_BLACKCAT : LinuxMalware
{
	meta:
		description = "Detect Linux version of BlackCat Ransomware"
		author = "Jesper Mikkelsen"
		reference = "https://www.virustotal.com/gui/file/056d28621dca8990caf159f8e14069a2343b48146473d2ac586ca9a51dfbbba7"
		date = "2022-05-10"
        yarahub_reference_md5 = "c7e39ead7df59e09be30f8c3ffbf4d28"
        yarahub_uuid = "4354fe5a-ee0c-47e3-a595-2824dd82928d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
		techniques = "File and Directory Permissions Modification: Linux and Mac File and Directory Permissions Modification"
		tactic = "Defense Evasion"
		mitre_att = "T1222.002"
		sharing = "TLP:WHITE"
        dname = "Ransom.Linux.BLACKCAT.YXCDFZ"
		score = 75
	strings:
		$pattern0 = "sbin*/cdrom*/dev*/etc*/lib**lost+found*/proc*/run*/snap*/tmp*/sys*/usr*/bi"
		$pattern1 = "n `vim-cmd vmsvc/getallvms| awk '{print$1}'`;do vim-cmd vmsvc/sn"
		$pattern2 = { BB 6C EA 3F AA 84 31 C4 13 19 F2 
        	   4C 47 F1 29 B7 FE 88 43 CA EF 60 
               98 31 56 7A 97 30 CD 92 4C CB 74 
               EB 26 B6 65 03 FD 4D DC D1 A1 A7
               CC 39 7A 5C 75 40 10 21 64 A8 CB
               DA DD B2 C4 DB 46 5A 1F 20 }
		$pattern3 = { 78 15 58 9C 99 1A C5 47 BC 7B B9
        	   31 5D 74 24 C7 E9 E0 72 B1 08 EF
               EF 6A 2D 8E 93 1C CC 81 0E DC 66
               4C 6B AA 87 43 F6 71 A2 22 8A 07
               43 2D 17 9D CB 0B 27 EB 2A 04 BA
               30 0F 65 C6 46 EE 6A 5B 86 }
		$pattern4 = { 72 78 B3 93 0F 69 5B 48 F4 D0 89
        	   14 1E C0 61 CF E5 79 18 A5 98 68
               F0 7E 63 D1 EA 71 62 4A 02 AA 99 
               F3 7B C0 E4 E2 93 1B 1F 5B 0E D8 
               97 0F E6 03 6C B6 9F 69 11 A7 77 
               B2 EA 1E 6D BD EB 85 85 66 }
		$pattern5 = { 39 67 68 97 DB 59 03 55 34 5A B8 
        	   62 DF 64 D3 A0 30 D1 0A 58 A9 EF 
               61 9A 46 EC DA AD AD D2 B1 6F 42 
               AB AA B3 A0 95 C1 71 4F 96 7A 46 
               A4 A8 11 84 4B 25 4A 8F BA 1B 21 
               4D 55 18 9A 7A BE 26 F1 B8 51 }
		$pattern6 = { 4B 35 35 C4 3D D4 3A 59 A7 5C 1C 
        	   69 D1 BD 13 F4 0A 98 72 88 7C 79 
               7D 15 BC D3 B0 70 CA 32 BF ED 11 
               17 DE 91 67 F6 D1 0C 91 42 45 5A 
               E7 A3 4A C7 3C 86 2B BB 4A 67 24 
               26 8A CD E9 43 FC 2C E6 DE 27 09 
               87 A2 51 E8 88 3F }
		$pattern7 = { 6B DA AE D5 B0 21 17 CF BF 20 8C 
        	   27 64 DB 35 5E 0E A6 24 B6 D5 5D 
               9D 2B 16 D5 C9 C3 CD 2E 70 BA A7 
               53 61 52 7C A8 D8 48 73 A9 43 A0 
               A8 52 FA D9 C2 2F EB 31 19 D4 52 
               BB F0 87 4E 53 2B 7C F7 2A 41 01 
               E6 C2 9A FA 5F D8 95 FB C4 }
	condition:
		all of them
}