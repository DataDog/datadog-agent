rule gorilla_bot
{
    meta:
        description = "Detects GorillaBot runtime strings"
        author = "asyncthecat"
        date = "2025-11-08"
        yarahub_author_twitter = "@asyncthecat"
        yarahub_author_email = "asyncthecat@mailhaven.su"
        yarahub_reference_md5 = "a650c998b6d2272aa51314461d7949ef"
        yarahub_uuid = "ea5f03a1-3ab6-4bbe-b8b0-a643d92a6776"
        yarahub_license = "CC BY-SA 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $s1 = "Found And Killed Process: PID=%d, Realpath=%s" ascii
        $s2 = "arm.nn" ascii
        $s3 = "arm5.nn" ascii
        $s4 = "arm6.nn" ascii
        $s5 = "m68k.nn" ascii
        $s6 = "mips.nn" ascii
        $s7 = "mipsel.nn" ascii
        $s8 = "powerpc.nn" ascii
        $s9 = "sparc.nn" ascii

    condition:
        any of ($s*)
}
