"""Lookup of standard license texts by license id.

Port of `STANDARD_LICENSES` from omnibus-ruby `lib/omnibus/licensing.rb`. Omnibus maps
each id to `https://dd-agent-omnibus.s3.amazonaws.com/licenses/<name>`; here `<name>` is
a file under `text/`, snapshotted from that bucket. See README.md for provenance and known
issues.
"""

# License id -> file name under text/ (the basename of the omnibus S3 URL).
STANDARD_LICENSES = {
    # Below licenses are compiled based on https://opensource.org/licenses/alphabetical
    "AFL-3.0": "AFL-3.0",  # Academic Free License 3.0
    "APL-1.0": "APL-1.0",  # Adaptive Public License
    "Apache-2.0": "Apache-2.0",  # Apache License 2.0
    "APSL-2.0": "APSL-2.0",  # Apple Public Source License
    "Artistic-2.0": "Artistic-2.0",  # Artistic license 2.0
    "AAL": "AALAAL",  # Attribution Assurance Licenses
    "BSD-3-Clause": "BSD-3-Clause",  # BSD 3-Clause "New" or "Revised" License
    "BSD-2-Clause": "BSD-2-Clause",  # BSD 2-Clause "Simplified" or "FreeBSD" License
    "BSD-1-Clause": "BSD-1-Clause",  # BSD 1-Clause
    "BSL-1.0": "BSL-1.0",  # Boost Software License
    "CECILL-2.1": "CECILL-2.1",  # CeCILL License 2.1
    "CATOSL-1.1": "CATOSL-1.1",  # Computer Associates Trusted Open Source License 1.1
    "CDDL-1.0": "CDDL-1.0",  # Common Development and Distribution License 1.0
    "CPAL-1.0": "CPAL-1.0",  # Common Public Attribution License 1.0
    "CUA-OPL-1.0": "CUA-OPL-1.0",  # CUA Office Public License Version 1.0
    "EUDatagrid": "EUDatagrid",  # EU DataGrid Software License
    "EPL-1.0": "EPL-1.0",  # Eclipse Public License 1.0
    "eCos-2.0": "eCos-2.0",  # eCos License version 2.0
    "ECL-2.0": "ECL-2.0",  # Educational Community License, Version 2.0
    "EFL-2.0": "EFL-2.0",  # Eiffel Forum License V2.0
    "Entessa": "Entessa",  # Entessa Public License
    "EUPL-1.1": "EUPL-1.1",  # European Union Public License, Version 1.1
    "Fair": "Fair",  # Fair License
    "Frameworx-1.0": "Frameworx-1.0",  # Frameworx License
    "FPL-1.0.0": "FPL-1.0.0",  # Free Public License 1.0.0
    "AGPL-3.0": "AGPL-3.0",  # GNU Affero General Public License v3
    "GPL-2.0": "GPL-2.0",  # GNU General Public License version 2.0
    "GPL-3.0": "GPL-3.0",  # GNU General Public License version 3.0
    "LGPL-2.0": "LGPL-2.0",  # GNU Library or "Lesser" General Public License version 2.0
    "LGPL-2.1": "LGPL-2.1",  # GNU Library or "Lesser" General Public License version 2.1
    "LGPL-3.0": "LGPL-3.0",  # GNU Library or "Lesser" General Public License version 3.0
    "HPND": "HPND",  # Historical Permission Notice and Disclaimer
    "IPL-1.0": "IPL-1.0",  # IBM Public License 1.0
    "IPA": "IPA",  # IPA Font License
    "ISC": "ISC",  # ISC License
    "LPPL-1.3c": "LPPL-1.3c",  # LaTeX Project Public License 1.3c
    "LiLiQ-P": "LiLiQ-P",  # Licence Libre du Quebec Permissive
    "LiLiQ-R": "LiLiQ-R",  # Licence Libre du Quebec Reciprocite
    "LiLiQ-R+": "LiLiQ-R+",  # Licence Libre du Quebec Reciprocite forte
    "LPL-1.02": "LPL-1.02",  # Lucent Public License Version 1.02
    "MirOS": "MirOS",  # MirOS Licence
    "MS-PL": "MS-PL",  # Microsoft Public License
    "MS-RL": "MS-RL",  # Microsoft Reciprocal License
    "MIT": "MIT",  # MIT license
    "Motosoto": "Motosoto",  # Motosoto License
    "MPL-2.0": "MPL-2.0",  # Mozilla Public License 2.0
    "Multics": "Multics",  # Multics License
    "NASA-1.3": "NASA-1.3",  # NASA Open Source Agreement 1.3
    "NTP": "NTP",  # NTP License
    "Naumen": "Naumen",  # Naumen Public License
    "NGPL": "NGPL",  # Nethack General Public License
    "Nokia": "Nokia",  # Nokia Open Source License
    "NPOSL-3.0": "NPOSL-3.0",  # Non-Profit Open Software License 3.0
    "OCLC-2.0": "OCLC-2.0",  # OCLC Research Public License 2.0
    "OGTSL": "OGTSL",  # Open Group Test Suite License
    "OSL-3.0": "OSL-3.0",  # Open Software License 3.0
    "OPL-2.1": "OPL-2.1",  # OSET Public License version 2.1
    "PHP-3.0": "PHP-3.0",  # PHP License 3.0
    "PostgreSQL": "PostgreSQL",  # The PostgreSQL License
    "Python-2.0": "Python-2.0",  # Python License
    "CNRI-Python": "CNRI-Python",  # CNRI Python license
    "QPL-1.0": "QPL-1.0",  # Q Public License
    "RPSL-1.0": "RPSL-1.0",  # RealNetworks Public Source License V1.0
    "RPL-1.5": "RPL-1.5",  # Reciprocal Public License 1.5
    "RSCPL": "RSCPL",  # Ricoh Source Code Public License
    "OFL-1.1": "OFL-1.1",  # SIL Open Font License 1.1
    "SimPL-2.0": "SimPL-2.0",  # Simple Public License 2.0
    "Sleepycat": "Sleepycat",  # Sleepycat License
    "SPL-1.0": "SPL-1.0",  # Sun Public License 1.0
    "Watcom-1.0": "Watcom-1.0",  # Sybase Open Watcom Public License 1.0
    "NCSA": "NCSA",  # University of Illinois/NCSA Open Source License
    "UPL": "UPL",  # Universal Permissive License
    "VSL-1.0": "VSL-1.0",  # Vovida Software License v. 1.0
    "W3C": "W3C",  # W3C License
    "WXwindows": "WXwindows",  # wxWindows Library License
    "Xnet": "Xnet",  # X.Net License
    "0BSD": "0BSD",  # Zero Clause BSD License
    "ZPL-2.0": "ZPL-2.0",  # Zope Public License 2.0
    "Zlib": "Zlib",  # zlib/libpng license

    # In addition to these we would like to add some of the licenses that
    # are frequently used in our dependencies.
    "Public-Domain": "Public-Domain",
    "Ruby": "Ruby",
    "Erlang-Public": "Erlang-Public",
    "Oracle-Binary": "Oracle-Binary",
    "OpenSSL": "OpenSSL",
    "Chef-MLSA": "Chef-MLSA",

    # Not in omnibus. Ids used in LICENSE-3rdparty.csv that omnibus could not resolve, so
    # it shipped no text for them.
    "UPL-1.0": "UPL",
    "Public Domain": "Public-Domain",
    "BSD-2-Clause-Views": "BSD-2-Clause-Views",  # From SPDX license-list-data v3.27.0.
}

def standard_license(license_id):
    """Returns the label of the standard license text for `license_id`.

    Unlike omnibus, which only warned, an unknown id is an error.

    Args:
      license_id: license id, e.g. "Apache-2.0".

    Returns:
      Label of a filegroup holding the license text.
    """
    name = STANDARD_LICENSES.get(license_id)
    if not name:
        fail("No standard license text for '%s'. Known ids: %s" % (license_id, ", ".join(sorted(STANDARD_LICENSES))))
    return Label("//third_party/licenses:" + name)
