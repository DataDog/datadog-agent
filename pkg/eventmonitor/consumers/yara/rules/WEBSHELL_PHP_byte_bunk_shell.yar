rule WEBSHELL_PHP_byte_bunk_shell
{
    meta:
        author                    = "Efrain Gutierrez"
        description               = "BYTE_BUNK Shell v4.0 - PHP webshell with OS command execution, MySQL access and a reverse-shell socket, distributed as the root index.php of a repackaged copy of the legitimate WordPress plugin Protect Uploads (Alticreation). Probes for a surviving exec function and clears open_basedir, disable_functions, safe_mode and suhosin.executor.disable_eval."
        date                      = "2026-09-23"
        yarahub_reference_link    = "https://www.virustotal.com/gui/file/63de5b3b03e3fb95e4d736ce0bec76e54c5299a0553b310ed3b9f270df841a1e"
        yarahub_reference_md5     = "48fa2e031c30b31e0bd7f4e53680ea07"
        yarahub_uuid              = "80b4b091-ffb9-401c-8df3-5405b99315c7"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

        hash_payload = "64c175284dc211529b811795c61ba5b8e5f30d162ea7f15845edae945a6fd9a4"
        hash_archive = "63de5b3b03e3fb95e4d736ce0bec76e54c5299a0553b310ed3b9f270df841a1e"
        detection    = "0/63 on VirusTotal, unchanged over four months"
        actor        = "Self-attributed to BYTE_BUNK, Telegram @Eagle0799"
        source       = "Captured by a WordPress login honeypot on ccbeautystudios.com, 2026-09-23 04:06:27 UTC, uploaded via /wp-admin/update.php?action=upload-plugin. Never executed."
        related      = "Same operator and same plugin disguise as WEBSHELL_PHP_dark_websocket_filemanager, but a distinct and more capable tool."

    strings:
        $php = "<?php"

        // Self-attribution. Two forms so dropping the version does not help.
        $id1 = "BYTE_BUNK" ascii nocase
        $id2 = "Advanced Bypassable Web Shell" ascii nocase
        $id3 = "@Eagle0799" ascii

        // Hardening bypasses. The suhosin line in particular is rare in
        // legitimate code and is switched off here on load.
        $byp1 = "suhosin.executor.disable_eval" ascii
        $byp2 = "ini_set('open_basedir', NULL)" ascii
        $byp3 = "ini_set('disable_functions', '')" ascii
        $byp4 = "$_GET['bypass']" ascii

        // The exec-function probe: it walks this list and uses whichever the
        // host has not disabled.
        $exec = "'system', 'exec', 'shell_exec', 'passthru', 'popen', 'proc_open', 'pcntl_exec'" ascii

        // Helper functions defined by this shell.
        $fn1 = "function getWorkingFunction(" ascii
        $fn2 = "function resolvePath(" ascii
        $fn3 = "function executeCommand(" ascii
        $fn4 = "function readContent(" ascii
        $fn5 = "function writeContent(" ascii
        $fn6 = "function scanPath(" ascii
        $fn7 = "function deleteItem(" ascii
        $fn8 = "function isWritableEnhanced(" ascii
        $fn9 = "function getSystemInfo(" ascii

    condition:
        $php
        and filesize < 400KB
        and (
            // named build
            any of ($id*)

            // rebranded: the exec probe plus its own helpers
            or ( $exec and 3 of ($fn*) )

            // rebranded and reordered: the bypass set plus its helpers
            or ( 3 of ($byp*) and 4 of ($fn*) )

            // helper set alone is distinctive enough in volume
            or ( 6 of ($fn*) )
        )
}
