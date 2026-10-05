/*
  Yara created from Chinese nc.exe strings
*/
rule Chinese_APT_Backdoor
{
	meta: 
		date = "2023-09-11"
		yarahub_uuid = "b11b03a5-e30b-4587-bd53-77f5202dae09"
		yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c90459986070e38fd8260d4430e23dfd"
		author = "schmidtsz"
		description = "Identify Chinese APT Backdoor"
		
  strings:
    $0 = "_getportpoop"
    $1 = "_portpoop"
    $2 = "_gethostpoop"
    $3 = "_ding2"
	$4 = "_ding1"
	$5 = "_o_alla"
	$6 = "_holler"
	
  condition:
	all of them
}