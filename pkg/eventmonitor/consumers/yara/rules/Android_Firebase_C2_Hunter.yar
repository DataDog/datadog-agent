rule Android_Firebase_C2_Hunter {
    meta:
        author = "SecurityResearcher"
        description = "Detects Android APK with hardcoded Firebase RTDB C2 endpoints"
        date = "2026-09-21"
        yarahub_reference_md5 = "e3b0c44298fc1c149afb4c8996fb9242"
        yarahub_uuid = "fa285e7c-6a1c-4041-85df-db6caf5a1baa"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $zip_header = { 50 4B 03 04 }
        $firebase_url = "firebaseio.com" ascii wide
        $firebase_key = /AIzaSy[A-Za-z0-9_-]{35}/ ascii wide

    condition:
        $zip_header at 0 and all of ($firebase_*)
}