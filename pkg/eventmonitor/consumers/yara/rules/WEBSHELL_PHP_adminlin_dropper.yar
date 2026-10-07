rule WEBSHELL_PHP_adminlin_dropper
{
    meta:
        author                    = "Efrain Gutierrez"
        description               = "CMSmap WordPress admin-account dropper. Bootstraps WordPress via wp-blog-header.php, inserts an administrator named adminlin directly into the users table with a hardcoded MD5 password, escalates it with set_role, and echoes the site URL back to the operator. Distributed as a one-file plugin archive with a randomised seven-letter name."
        date                      = "2026-09-23"
        yarahub_reference_link    = "https://threatfox.abuse.ch/browse.php?search=ioc%3A9db1f0f3fbaabb644021c1d872619ad53ade664f8b9794bb9b64fda013f93798"
        yarahub_reference_md5     = "fbe7429bc7c2478efcb7c888b2931479"
        yarahub_uuid              = "5edc9700-05be-4027-a299-727fac4509ae"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        hash_payload = "9db1f0f3fbaabb644021c1d872619ad53ade664f8b9794bb9b64fda013f93798"
        hash_archive = "4e2a6a49b22ebee5fc155eb145e02f5c13b0f8355680fbcbbeae9ac554110b7c"
        detection    = "0/62 on VirusTotal; present there since approximately 2021, byte-identical"
        source       = "Captured by a WordPress login honeypot on hairbyjulietsalon.com, September 2026, uploaded via /wp-admin/update.php?action=upload-plugin. Never executed."
        note         = "The rogue account is the part that matters - it survives deleting plugins, cleaning files and restoring a theme, and looks ordinary in the users list."

    strings:
        $php = "<?php"

        // The account it creates, and the plugin header it ships with.
        $id1 = "$user_loginv = 'adminlin'" ascii
        $id2 = "adminlin" ascii
        $hdr = "CMSmap - WordPress Shell" ascii

        // Hardcoded credential material. WordPress still accepts a bare
        // 32-character MD5 and upgrades it on first login.
        $pw   = "57a48cf5883989417e6c0583c87ceb40" ascii
        $date = "'user_registered' =>'2012-08-03 01:24:01'" ascii
        $mail = "'user_email' =>'admin@admin.com'" ascii

        // Mechanism: bootstrap WordPress, insert the row, escalate, report back.
        $boot  = "include($path.'wp-blog-header.php')" ascii
        $ins   = "$wpdb->insert($table_prefix.'users'" ascii
        $role  = "set_role( 'administrator' )" ascii
        $exfil = "echo $siteurl->option_value.'|'.$user_loginv.'|';" ascii

    condition:
        $php
        and filesize < 50KB
        and (
            // named build
            $id1 or $hdr

            // renamed account: the hardcoded password hash beside a direct
            // insert into the users table is specific on its own
            or ( $pw and $ins )

            // rebuilt: bootstrap, insert, escalate and phone home together
            or ( $boot and $ins and $role and $exfil )

            // rebuilt with the same fabricated registration details
            or ( $date and $mail and $ins )

            // account name still present alongside the insert and escalation
            or ( $id2 and $ins and $role )
        )
}
