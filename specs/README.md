# Remote packet capture — spEARS v2 case study

A retrospective specification of [datadog-agent PR #57782](https://github.com/DataDog/datadog-agent/pull/57782).
The source revision inspected is `5190445c44de7544b67072705218f4af2dd4a1b5`.
This branch adds the specification documents on top of that revision; it does
not update the original PR branch.

## Read the layers

- **Goal:** Let network operators obtain bounded packet-header evidence from a
  host without shell access, while avoiding application payload collection.
- **Requirements:** [Eleven EARS requirements](remote-packet-capture/requirements.md)
  capture observable behavior and operator value.
- **Decisions:** [Three retrospective ADRs](adrs/README.md) reconstruct real
  review exchanges about hard capture ceilings, fragment headers, and bounded
  teardown. These records are sourced reconstructions, not newly approved
  project decisions.
- **Precise behavior:** [Allium lifecycle model](remote-packet-capture/remote-packet-capture.allium)
  describes capture, stream acceptance, delivery, and cleanup attempts. Packet
  privacy and endpoint delivery are boundary obligations; the model does not
  duplicate the decoder or HTTP implementation.
- **Status:** [Executive summary](remote-packet-capture/executive.md) separates
  implementation evidence from verification gaps and open questions.
- **Traceability:** [Requirement-to-code/test map](traceability.md) links to
  source and existing test definitions at the inspected revision.

## A useful example of requirements evolving

The review identified that caller-supplied limits could bypass the defaults
and that retries held the full artifact in memory.
[The review concern](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4275837111)
and [author response](https://github.com/DataDog/datadog-agent/pull/57782#discussion_r4318738920)
show the move to enforceable ceilings and file-backed streaming retries.
The requirement states the standing output bound; ADR-001 records why that
contract and implementation changed.

A further review found that a consumer could stop reading while retaining the
connection, preventing teardown despite the capture duration expiring.
ADR-003 records the response-deadline decision. The Allium layer distinguishes
capture completion from stream acceptance and delivery success, and requires
a cleanup attempt after a temporary artifact has been used.

## Evidence and verification limits

The extraction used the PR description, full diff, 45 changed Go/YAML source
files, existing test definitions, and 30 inline review comments, retrieved on
October 22, 2026. Source/test links pin the inspected commit. Review comments
may concern older revisions and are interpreted against the pinned source.

Allium CLI 3.5.2 reported no structural diagnostics. Candidate test obligations
were derived with `allium plan`. No Agent builds, Go tests, privileged captures,
or live intake uploads were executed for this extraction. Structural checking
does not establish spec–code conformance or runtime correctness.

These are candidate specifications reconstructed from an existing PR.
Stakeholders have not approved their wording as the intended product contract.
Three model questions remain open: concurrent capture admission, absent or
malformed final statistics, and file limits below the mandatory PCAP header.

The local evidence archive and generated diagnostic files are deliberately
excluded from this branch. All new files are under this directory.
