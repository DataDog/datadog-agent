# Evidence and requirement traceability

Snapshot: PR [57782](https://github.com/DataDog/datadog-agent/pull/57782), head `5190445c44de7544b67072705218f4af2dd4a1b5`. Links below pin source to that head. Evidence consists of the PR description, full patch, changed source/tests, and review exchanges linked from the reconstructed ADRs. The evidence archive remains local and is not included on this branch. Existing upstream code and tests do not contain these local REQ IDs; this matrix is a sidecar mapping, not a claim of native grep-through-code traceability. No upstream edits or test execution were performed.

## Interpretation

- **Explicit intent:** the PR body or an author reply directly states the capability or reason.
- **Reconstructed contract:** observable behavior inferred from the pinned implementation and tests, reformulated into requirements. It is not evidence the author approved this wording.
- Rationales express operator value inferred for this case study unless a cited exchange establishes the motivation. The PR's explicit overall motivation is host troubleshooting without shell access.

## Requirement map

| Requirement | Intent basis | Implementation symbols | Existing test definitions inspected; remaining limit |
| --- | --- | --- | --- |
| REQ-PCAP-001 | Explicit remote troubleshooting/action/filter capability; default-interface/filter behavior reconstructed | `RunCaptureHandler.Run` [A]; `socketCaptureTrigger.Capture` [B]; `newCapturer`, `checkFilterSyntax`, `capturer.openHandle` [C]; `CaptureConfig.applyDefaults` [D] | `TestRunCapture_OmittedBPFFilterIsAccepted`, `TestRunCaptureValidation_ValidInputs` [AT]; `TestNewCapturer`, `TestApplyDefaults` [CT]; real action/socket tests [RT]. Real filter/interface path not executed. |
| REQ-PCAP-002 | Explicit disabled-by-default/build-tag gating; failure behavior reconstructed | `PacketCapture`, build constraint [E]; `New`, packet-capture enable branch [F]; schema `packet_capture.enabled` [G]; `socketCaptureTrigger.Capture` [B]; unsupported-platform `Capture` [S] | `TestSocketCaptureTrigger_ErrorStatus`, `TestSocketCaptureTrigger_SocketPathNotConfigured` [BT]. No direct opt-in/build-matrix assertion identified in changed tests. |
| REQ-PCAP-003 | Explicit header-only/fail-closed intent; fragment semantics author-confirmed in ADR-002 | `headerLayers`, `headerLen`, `fragmentHeaderLen`, `completeHeader` [H] | `TestHeaderLen`, `TestHeaderLenWithinSnapLen` [HT]; `TestPCAPOutputReadsBack` [OT]; fixtures cover application data, tunnels, fragments, malformed headers. Static fixtures are not a universal guarantee over every decoder input. |
| REQ-PCAP-004 | Explicit duration/ceilings; author confirms strengthened upper caps in ADR-001; header floor reconstructed | `RunCaptureHandler.Run` [A]; `boundRequest` [E]; `capturer.drainLoop`, file/header constants [C] | `TestRunCapture_CapsAlwaysBounded`, `TestRunCapture_ExplicitCapsArePreserved`, `TestRunCapture_LargerCapsAreLowered`, duration validation [AT]; `TestBoundRequest` [ET]; `TestMaxBytesAccounting`, `TestPCAPOutputReadsBack`, `TestPCAPSizeConstantsMatchWriter` [OT]. Packet-count stop lacks a dedicated changed test. The output/accounting tests exercise the writer and mirrored cap arithmetic, not the real drain loop. |
| REQ-PCAP-005 | Quiet-interface and stalled-writer failures evidenced by review; deadline fix author-confirmed in ADR-003 | `handleCapture`, `flushWriter`, `setWriteDeadline` [E]; `capturer.drainLoop`, `capturer.Stop` [C]; client timeout in `socketCaptureTrigger.Capture` [B] | `TestHandleCaptureSendsHeadersBeforeFirstPacket`, `TestHandleCaptureFlushDoesNotRaceWithPackets`, `TestSetWriteDeadline` [ET]; `TestDoneClosesWhenDurationElapses` [CT]. Setter test does not simulate stalled-reader teardown; root-dependent tests not run. |
| REQ-PCAP-006 | Explicit review/author correction for read errors; other failure semantics reconstructed | `handleCapture` [E]; `capturer.drainLoop` [C]; `socketCaptureTrigger.Capture` [B]; `RunCaptureHandler.Run` [A] | `TestDrainLoopEndsOnReadError` [DT]; `TestSocketCaptureTrigger_CaptureErrorsTrailer`, `TestSocketCaptureTrigger_ErrorStatus` [BT]; `TestRunCapture_UploadFailureFailsTheAction`, `TestRunCapture_NoUploaderFailsTheAction` [AT]. Missing/malformed trailers and Stop errors are not strict success gates. |
| REQ-PCAP-007 | Identifier-expiry and incomplete-capture reasons stated in source/test comments; result/metadata contract reconstructed | `RunCaptureInputs`, `RunCaptureResult`, `RunCaptureHandler.Run` [A]; `captureOutcome`, `trailerInt` [B]; `networkPcapEvent`, `uploadCapture` [I]; `CaptureStats` [D] | `TestRunCapture_CaptureIDIsHonoured`, `TestRunCapture_CaptureIDGeneratedWhenAbsent`, `TestRunCapture_ReportsCaptureCounts`, `TestRunCapture_UploadsCapture` [AT]; `TestSocketCaptureTrigger_Success` [BT]. Count-reporting tests use fake captures; hostname-failure fallback not directly asserted. |
| REQ-PCAP-008 | Explicit upload capability; cannot-retake reason in source comments; exact retry policy reconstructed | `uploadCapture` [I]; `uploader.Upload`, `uploader.sendWithRetries`, `uploader.send`, `newMultipartBody`, `writeMultipart` [J] | `TestRunCapture_UploadsCapture`, upload failure tests [AT]; `TestUploadSendsZstdMultipart`, `TestUploadRetryReopensTheSource`, `TestUploadRetriesRetryableStatuses`, `TestUploadDoesNotRetryTooLarge`, `TestUploadDoesNotRetryClientErrors`, `TestUploadGivesUpAfterMaxAttempts`, `TestUploadStopsRetryingWhenContextIsDone`, `TestUploadOpenFailureSendsNothing` [JT]. Opt-in real tests [JR]/[RT] not run. |
| REQ-PCAP-009 | Reconstructed from reusable forwarder and its tests/config; not presented as a separate customer ask | `uploader.getEndpoints`, `uploader.Upload`, `uploader.send`, `buildURL` [J]; `getNetworkPcapTracks`/track descriptor [K]; core `network_pcap.forwarder` settings [L] | `TestUploadAdditionalEndpoints`, `TestUploadMRFEndpointFollowsFailover`, `TestDefaultEndpointUsesTrackHostPrefix`, `TestEndpointOverrideKeepsPortAndPath`, `TestUploadWithoutAPIKeyFails` [JT]. Inspect full assertions before treating reliable/unreliable and runtime key refresh as tested end to end. |
| REQ-PCAP-010 | Explicit dedicated socket/isolation/nonfatal-start rationale in PR body; channel selection reconstructed | `newPacketCaptureHandler`, `startPacketCaptureServer` [M]; server integration [N]; `captureSocketPath` [B] | `TestPacketCaptureHandlerServesOnlyCaptureRoutes`, `TestPacketCaptureHandlerDoesNotEscapePrefix`, `TestStartPacketCaptureServer` [MT]; `TestCaptureSocketPath` [BT]. Configured dedicated socket is not retried via main socket after dial failure. |
| REQ-PCAP-011 | Reconstructed from cleanup paths and explicit removal assertions | `RunCaptureHandler.Run` deferred removal [A]; `socketCaptureTrigger.Capture` file error paths [B] | `TestRunCapture_UploadsCapture`, `TestRunCapture_UploadFailureFailsTheAction` [AT]; error-trailer test [BT] asserts no returned artifact path but does not independently scan disk for leaked files. Removal failure and process crashes are outside the demonstrated guarantee. |

## Optional behavioral layer

The [Allium model](remote-packet-capture/remote-packet-capture.allium) gives REQ-PCAP-004, REQ-PCAP-005, REQ-PCAP-006, REQ-PCAP-008, and REQ-PCAP-011 an explicit ordered lifecycle: `SubmitCapture` normalizes caps; `RejectInvalidDuration`/`BeginCapture` gate collection; `FailCapture` prevents delivery of rejected streams; `FinishCaptureStream` requires a finalized temporary artifact; `CompleteDelivery`/`FailDelivery` record delivery outcome and cleanup attempts. `EffectiveLimitsRemainBounded`, `SuccessfulActionWaitsForDelivery`, and `CompletedActionsAttemptTemporaryCleanup` state the corresponding model invariants. `HeaderOnlyCapture` and `CaptureDelivery` annotate the REQ-PCAP-003/004/005 and REQ-PCAP-008/009 boundaries in prose; those annotations do not prove the packet decoder or uploader.

Allium 3.5.2 syntax/semantic checking passed. Planned test obligations were generated for local review (the generated file remains in the local evidence archive); they are not executable tests and were not run against Agent code. No spec–code divergence checker was run. Three explicit open questions remain in the model, so the behavioral layer is a reviewable retrospective draft rather than a fully resolved implementation contract.

## Scope across the full patch


The feature requirements cover the capture engine, system-probe endpoint and dedicated socket, action runner, temporary-file lifecycle, new fileupload component, and delivery configuration. Remaining changed files wire these paths into constructors, bundle registries, module registration, build targets, generated configuration schema, component inventory, and ownership. Those changes support the mapped capabilities; they do not automatically create distinct user requirements. No frontend, backend retrieval implementation, remote-config capture workflow, or production deployment policy is inferred.

## Pinned source and test references

[A]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture.go
[AT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture_test.go
[B]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture_socket.go
[BT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture_socket_test.go
[C]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/capture.go
[CT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/capture_test.go
[D]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/config.go
[DT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/drain_test.go
[E]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/cmd/system-probe/modules/packet_capture.go
[ET]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/cmd/system-probe/modules/packet_capture_test.go
[F]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/system-probe/config/config.go
[G]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/config/schema/yaml/system-probe_schema.yaml
[H]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/trim.go
[HT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/trim_test.go
[I]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/upload.go
[J]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/comp/forwarder/fileupload/impl/fileupload.go
[JT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/comp/forwarder/fileupload/impl/fileupload_test.go
[JR]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/comp/forwarder/fileupload/impl/fileupload_real_test.go
[K]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/comp/forwarder/fileupload/impl/tracks_networkpcap.go
[L]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/config/schema/yaml/core_schema.yaml
[M]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/cmd/system-probe/api/packet_capture_socket.go
[MT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/cmd/system-probe/api/packet_capture_socket_test.go
[N]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/cmd/system-probe/api/server.go
[OT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/network/capture/pcap_output_test.go
[RT]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture_e2e_real_test.go
[S]: https://github.com/DataDog/datadog-agent/blob/5190445c44de7544b67072705218f4af2dd4a1b5/pkg/privateactionrunner/bundles/remoteaction/pcap/run_capture_stub.go
