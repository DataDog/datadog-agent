import "elf"

rule ELF_Backdoor_ZipLine_Feb2024 {
    meta:
        Description = "Detects Zipline backdoor malware samples based on ELF properties and strings"
        author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
        Credits = "Is Now on VT! for the notification of the malware sample"
        Reference = "https://www.mandiant.com/resources/blog/investigating-ivanti-zero-day-exploitation"
        Hash = "8cad755bcf420135c0f406fb92138dcb0c1602bf72c15ed725bd3b76062dafe5"
        date = "2024-02-19"
        yarahub_author_twitter = "@RustyNoob619"
        yarahub_reference_md5 = "b4a31fa229cd1074c5cbd1c84a01c6ae"
        yarahub_uuid = "30c0e5bf-db74-458d-b2a4-7bc2f43e0463"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    
    strings:
    $dir1 = "/tmp/data/root/home/lib/%s"
    $dir2 = "/tmp/data/root/etc/ld.so.preload"
    $dir3 = "/tmp/data/root/home/etc/manifest/exclusion_list"
    $dir4 = "/proc/self/exe"
    $dir5 = "/proc/self/cmdline"
    $dir6 = "/home/etc/manifest/exclusion_list"

    $cmd1 = "./installer/bom_files"
    $cmd2 = "./installer/scripts"
    $cmd3 = "/retval=$(exec $installer $@)/d' /pkg/do-install"

    $sig = "SSH-2.0-OpenSSH_0.3xx"

    condition:
       for 3 sym in elf.dynsym:
       (sym.name == "_ITM_deregisterTMCloneTable" 
       or sym.name == "_ITM_registerTMCloneTable" 
       or sym.name == "__cxa_finalize")
       and 3 of ($dir*)
       and any of ($cmd*) 
       and $sig
       
 }


 


  