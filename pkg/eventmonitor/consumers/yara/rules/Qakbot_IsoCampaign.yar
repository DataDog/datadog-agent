rule Qakbot_IsoCampaign{
meta:
author = "Malhuters"
description = "Qakbot New Campaign ISO"
date = "2022-10-06"
yarahub_reference_md5 = "456373BC4955E0B6750E8791AB84F004"
yarahub_uuid = "cef91a6a-f270-4c35-87a4-98b6f78096db"
yarahub_license = "CC0 1.0"
yarahub_rule_matching_tlp = "TLP:WHITE"
yarahub_rule_sharing_tlp = "TLP:WHITE"
malpedia_family = "win.qakbot"
strings:
$str1 = "CD001"
$str2 = "This disc contains Unicode file names and requires an operating system"
$str3 = "such as Microsoft Windows 95 or Microsoft Windows NT 4.0."
$str4 = "README.TXT"
$str5 = "Windows"
$str6 = "C:\\Windows\\System32\\cmd.exe"
$str7 = "%SystemRoot%\\System32\\shell32.dll"
$str8 = "desktop-"
$str9 = ">CREATOR: gd-jpeg v1.0 (using IJG JPEG v62), default quality"
condition:
(5 of ($str*)) 
}