rule INC_Ransom_Note_INC_README
{
    meta:
        description                = "Detects the INC Ransom ransom note (INC-README.txt, or /etc/motd replaced by it)"
        author                     = "Peter Gabaldon"
        date                       = "2026-09-18"
        yarahub_uuid               = "f4ec2a96-0524-4bac-bd7b-433c03328d60"
        yarahub_license            = "CC0 1.0"
        yarahub_rule_matching_tlp  = "TLP:WHITE"
        yarahub_rule_sharing_tlp   = "TLP:WHITE"
        yarahub_reference_md5      = "3d584fc510e5450b77ecdeebb708e1f1"

    strings:
        $hdr  = "~~~~ INC Ransom ~~~~" ascii
        $id   = "Your personal ID:" ascii
        $t1   = "Your data is stolen and encrypted." ascii
        $t2   = "Don't go to recovery companies!" ascii
        $t3   = "we will attack your company again in the future." ascii

    condition:
        // anchored at offset 0 and size-bounded so that threat-intel reports
        // quoting the note do not match
        $hdr at 0 and filesize < 16KB and $id and 2 of ( $t* )
}
