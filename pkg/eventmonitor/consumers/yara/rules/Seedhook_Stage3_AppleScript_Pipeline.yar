rule Seedhook_Stage3_AppleScript_Pipeline
{
    meta:
        yarahub_uuid = "98f82c16-4299-4e91-8b33-c941ecdc7df1"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "bef4d496763a034c1e0af22a65c6f212"
        description = "Seedhook/MacSync stage-3 AppleScript stealer: /pipeline/event telemetry fields, fromCodes() ASCII-array string builder, AMOS-derived handler names"
        actor = "Seedhook (MacSync Stealer cluster)"
        reference_sha256 = "b647c8189ad6305e940bd904e496bde966cdcba333e9044a1567cf98d707d959"
        date = "2026-09-19"
        tlp = "CLEAR"

    strings:
        // client-side telemetry to <host>/pipeline/event (multipart fields + custom headers on /gate)
        $pipe1 = "/pipeline/event\"" ascii
        $pipe2 = "buildtxd=" ascii
        $pipe3 = "X-Log-Merge-Id: " ascii
        $pipe4 = "X-Pipeline-Session: " ascii
        $pipe5 = "stage=script_start" ascii
        $pipe6 = "X-Log-Part: " ascii
        // fake-dialog strings are assembled from ASCII code lists at run time
        $fc1 = "on fromCodes(cs)" ascii
        $fc2 = "(ASCII character n)" ascii
        // handler / template names
        $as1 = "on Filegrabber(writemind)" ascii
        $as2 = "on FilegrabberFDANotes(writemind, profile)" ascii
        $as3 = "NSSecureTextField" ascii
        $as4 = "writeText(\"Build Tag: " ascii
        $as5 = "on askPass(ttl, msg)" ascii
        $as6 = "on readwriteSafe(" ascii
        // known fake-dialog title code arrays: "Verify User Integrity" (safeguard), "System Preferences" (Build 10)
        $dlg1 = "{86, 101, 114, 105, 102, 121, 32, 85, 115, 101, 114, 32, 73, 110, 116, 101, 103, 114, 105, 116, 121}" ascii
        $dlg2 = "{83, 121, 115, 116, 101, 109, 32, 80, 114, 101, 102, 101, 114, 101, 110, 99, 101, 115}" ascii

    condition:
        filesize < 1MB and
        (
            (3 of ($pipe*) and 1 of ($fc*) and 2 of ($as*))
            or (1 of ($dlg*) and 1 of ($fc*) and 1 of ($as*))
        )
}
