rule WEBSHELL_PHP_dark_websocket_filemanager
{
    meta:
        author                    = "Efrain Gutierrez"
        description               = "Dark WEBSOCKET File Manager - unauthenticated PHP webshell giving full filesystem control, distributed as the root index.php of a repackaged copy of the legitimate WordPress plugin Protect Uploads (Alticreation). Clears open_basedir and disable_functions on load to escape shared-hosting restrictions."
        date                      = "2026-09-22"
        yarahub_reference_link    = "https://www.virustotal.com/gui/file/430956dbebb4bfde83d55ff57c6de288eccf557d36f552b1b2f7bbca94a87678"
        yarahub_reference_md5     = "f4cfad06bd0f889d6d7c113ffda27d15"
        yarahub_uuid              = "5be8fffa-685b-4127-afa2-0410285b6d5b"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        hash_payload = "e8dc6ca549b0aed1513ba3a4285556bca1c237e9608f51623b56ec03813403df"
        hash_archive = "430956dbebb4bfde83d55ff57c6de288eccf557d36f552b1b2f7bbca94a87678"
        detection    = "0/65 on VirusTotal at time of writing"
        source       = "Captured by a WordPress login honeypot on ccbeautystudios.com, 2026-09-22 19:27:49 UTC, uploaded via /wp-admin/update.php?action=upload-plugin. Never executed."

    strings:
        $php = "<?php"

        // Self-identifying banner. Kept in two lengths so dropping the
        // "Dark " prefix does not defeat the rule.
        $banner1 = "Dark WEBSOCKET File Manager" ascii nocase
        $banner2 = "WEBSOCKET File Manager" ascii nocase

        // The author's own "// Bypass" block: escape the PHP directory jail
        // and re-enable disabled functions. Distinctive as a pair.
        $bypass1 = "ini_set('open_basedir', NULL)" ascii
        $bypass2 = "ini_set('disable_functions', '')" ascii

        // Helper functions defined by this shell.
        $fn1 = "function writeFile(" ascii
        $fn2 = "function readFileContent(" ascii
        $fn3 = "function scanDirectory(" ascii
        $fn4 = "function newFile(" ascii
        $fn5 = "function newFolder(" ascii
        $fn6 = "function renameItem(" ascii
        $fn7 = "function changePermissions(" ascii

        // Request parameters its control panel drives.
        $p1 = "chmod_item" ascii
        $p2 = "chmod_value" ascii
        $p3 = "octdec($_POST['chmod_value'])" ascii

    condition:
        $php
        and filesize < 200KB
        and (
            // named build
            any of ($banner*)

            // renamed build: the sandbox escape plus its own helpers
            or ( all of ($bypass*) and 3 of ($fn*) )

            // rewritten header: the helper set plus its chmod plumbing
            or ( 5 of ($fn*) and 2 of ($p*) )
        )
}
