rule RANSOM_ESXiArgs_Ransomware_Bash_Feb23
{
    meta:
	author = "SECUINFRA Falcon Team (@SI_FalconTeam)"
	description = "Detects the ESXiArgs Ransomware encryption bash script"
	reference = "https://www.secuinfra.com/en/techtalk/hide-your-hypervisor-analysis-of-esxiargs-ransomware/"
	date = "2023-02-07"
	tlp = "CLEAR"
	yarahub_reference_md5 = "d0d36f169f1458806053aae482af5010"
	yarahub_uuid = "4498d57f-44ec-47f2-8455-ceeacd3dc07e"
	yarahub_license = "CC BY 4.0"
	yarahub_rule_matching_tlp = "TLP:WHITE"
	yarahub_rule_sharing_tlp = "TLP:WHITE"
	yarahub_author_twitter = "@SI_FalconTeam"

    strings:
	$bash = "#!/bin/sh"
	
	$wait = "Waiting for task' completion..."

	$comment0 = "## SSH HI"
	$comment1 = "## CHANGE CONFIG"
	$comment2 = "## STOP VMX"
	
	$kill0 = "echo \"KILL VMX\""
	$kill1 = "kill -9 $(ps | grep vmx | awk '{print $2}')"
	
	$index = "$path_to_ui/index1.html"

	$ext0 = ".vmdk" 
	$ext1 = ".vmx"
	$ext2 = ".vmxf"
	$ext3 = ".vmsd"
	$ext4 = ".vmsn"
	$ext5 = ".vswp"
	$ext6 = ".vmss"
	$ext7 = ".nvram"
	$ext8 = ".vmem"

	$clean0 ="/bin/rm -f $CLEAN_DIR\"encrypt\" $CLEAN_DIR\"nohup.out\" $CLEAN_DIR\"index.html\" $CLEAN_DIR\"motd\" $CLEAN_DIR\"public.pem\" $CLEAN_DIR\"archieve.zip\""
	$clean1 = "/bin/echo '' > /etc/rc.local.d/local.sh"

    condition:
	$bash
	and $wait
	and any of ($comment*)
	and 2 of ($kill*)
	and $index
	and 4 of ($ext*)
	and 2 of ($clean*)
}