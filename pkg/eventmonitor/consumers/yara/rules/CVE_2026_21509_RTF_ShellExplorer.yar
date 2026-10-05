rule CVE_2026_21509_RTF_ShellExplorer
{
    meta:
        description = "Detect RTF exploiting CVE-2026-21509 via Shell.Explorer.1 OLE object"
        cve = "CVE-2026-21509"
        exploit_primitive = "Shell.Explorer.1 OLE allowlist gap"
        technique = "RTF embedded OLE -> Shell.Explorer.1 -> Navigate()"
        delivery = "Remote LNK"
        actor = "APT28 / Others"
        confidence = "high"
        author = "Robin Dost"
        reference = "https://blog.synapticsystems.de/apt28-geofencing-as-a-targeting-signal-cve-2026-21509/"
        date = "2026-02-03"
        notes = "Valid OLE object. Exploit relies on allowlist gap."
        yarahub_author_twitter = "@Mr128BitSec"
        yarahub_author_email = "robin.dost@synapticsystems.de"
        yarahub_reference_md5 = "4727582023cd8071a6f388ea3ba2feaa"
        yarahub_uuid = "bf2bf9db-ab13-4138-993d-bffcac1b84fc"
        yarahub_license = "CC BY-SA 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $rtf = "{\\rtf" // is rtf?
        $objocx = "\\objocx"
        $objclass = "Word.Document.12" ascii
        $shell_hex = "C32AB2EAC130CF11A7EB0000C05BAE0B" ascii // detect guid
    
    condition:
        $rtf and
        $objocx and
        $objclass and
        $shell_hex
}

