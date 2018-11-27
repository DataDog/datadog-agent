// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package windowsevent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/clbanning/mxj"
)

const (
	binaryPath = "Event.EventData.Binary"
	dataPath   = "Event.EventData.Data"
	taskPath   = "Event.System.Task"
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
}

// eventContext links go and c
type eventContext struct {
	id int
}

// Tailer collects logs from event log.
type Tailer struct {
	source     *config.LogSource
	config     *Config
	outputChan chan *message.Message
	stop       chan struct{}
	done       chan struct{}

	context *eventContext
}

// NewTailer returns a new tailer.
func NewTailer(source *config.LogSource, config *Config, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		config:     config,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Identifier returns a string that uniquely identifies a source
func Identifier(channelPath, query string) string {
	return fmt.Sprintf("eventlog:%s;%s", channelPath, query)
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return Identifier(t.config.ChannelPath, t.config.Query)
}

// toMessage converts an XML message into json
func (t *Tailer) toMessage(event string) (*message.Message, error) {
	log.Debug("Rendered XML:", event)
	mxj.PrependAttrWithHyphen(false)
	mv, err := mxj.NewMapXml([]byte(event))
	if err != nil {
		return &message.Message{}, err
	}
	mv = remapDataField(mv)
	mv = replaceBinaryData(mv)
	if strings.HasPrefix(t.config.ChannelPath, "Microsoft-ServiceFabric/") {
		mv = mapTaskIDToTaskName(mv)
	}
	jsonEvent, err := mv.Json(false)
	if err != nil {
		return &message.Message{}, err
	}
	log.Debug("Sending JSON:", string(jsonEvent))
	return message.NewMessage(jsonEvent, message.NewOrigin(t.source), message.StatusInfo), nil
}

// mapTaskIDToTaskName looks for the TASK_ID in {"Event": {"System": {"Task": <TASK_ID> }}}
// and maps it to the name of the task that match in the Microsoft Task Codes
func mapTaskIDToTaskName(mv mxj.Map) mxj.Map {
	values, err := mv.ValuesForPath(taskPath)
	if err != nil || len(values) == 0 {
		log.Debug("Could not find path:", taskPath)
		return mv
	}

	taskName, found := taskIDMapping[values[0]]
	if !found {
		log.Debug("Could not resolve task id:", values[0])
		return mv
	}

	_, err = mv.UpdateValuesForPath("Task:"+taskName, taskPath)
	if err != nil {
		log.Debugf("An error occurred updating %s: %s", taskPath, err)
	}
	return mv
}

// remapDataField transforms the fields parsed from <Data Name='NAME1'>VALUE1</Data><Data Name='NAME2'>VALUE2</Data> to
// fields that will be JSON serialized to {"NAME1": "VALUE1", "NAME2": "VALUE2"}
// Data fields always have this schema:
// https://docs.microsoft.com/en-us/windows/desktop/WES/eventschema-complexdatatype-complextype
func remapDataField(mv mxj.Map) mxj.Map {
	values, err := mv.ValuesForPath(dataPath)
	if err != nil || len(values) == 0 {
		log.Debug("Could not find path:", dataPath)
		return mv
	}
	nameTextMap := make(map[string]interface{})
	for _, value := range values {
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		name, foundName := valueMap["Name"]
		text, foundText := valueMap["#text"]
		if !foundName || !foundText {
			continue
		}
		nameString, ok := name.(string)
		if !ok {
			continue
		}
		nameTextMap[nameString] = text
	}
	if len(nameTextMap) == 0 {
		return mv
	}
	err = mv.SetValueForPath(nameTextMap, dataPath)
	if err != nil {
		log.Debugf("Error formatting %s: %s", dataPath, err)
	}
	return mv
}

// replaceBinaryData remaps the field Event.EventData.Binary to its string value
func replaceBinaryData(mv mxj.Map) mxj.Map {
	values, err := mv.ValuesForPath(binaryPath)
	if err != nil || len(values) == 0 {
		log.Debug("Could not find path:", binaryPath)
		return mv
	}
	valueString, ok := values[0].(string)
	if !ok {
		log.Debug("Could not cast binary data to string:", err)
		return mv
	}

	decodedString, _ := parseBinaryData(valueString)
	if err != nil {
		log.Debugf("could not decode %s: %s", valueString, err)
		return mv
	}
	_, err = mv.UpdateValuesForPath("Binary:"+string(decodedString), binaryPath)
	if err != nil {
		log.Debugf("Error formatting %s: %s", binaryPath, err)
	}
	return mv
}

// parseBinaryData parses the hex string found in the field Event.EventData.Binary to an utf-8 valid string
func parseBinaryData(s string) (string, error) {
	// decoded is an utf-16 array of byte
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}

	// https://gist.github.com/bradleypeabody/185b1d7ed6c0c2ab6cec
	if len(b)%2 != 0 {
		return "", fmt.Errorf("Must have even length byte slice")
	}
	ret := &bytes.Buffer{}
	u16s := make([]uint16, 1)
	b8buf := make([]byte, 4)

	lb := len(b)
	for i := 0; i < lb; i += 2 {
		u16s[0] = uint16(b[i]) + (uint16(b[i+1]) << 8)
		r := utf16.Decode(u16s)
		n := utf8.EncodeRune(b8buf, r[0])
		ret.Write(b8buf[:n])
	}

	// The string might be utf16 null-terminated (2 null bytes)
	return strings.TrimRight(ret.String(), "\x00"), nil
}

// Mapping can be found here
// https://github.com/Microsoft/service-fabric/blob/c326b801c6c709f36684700edfe7bb88ceec9d7f/src/prod/src/Common/TraceTaskCodes.h
var taskIDMapping = map[interface{}]string{
	"1":   "Common",
	"2":   "Config",
	"3":   "Timer",
	"4":   "AsyncExclusiveLock",
	"5":   "PerfMonitor",
	"10":  "General",
	"11":  "FabricGateway",
	"12":  "Java",
	"13":  "Managed",
	"16":  "Transport",
	"17":  "IPC",
	"18":  "UnreliableTransport",
	"19":  "LeaseAgent",
	"25":  "ReliableMessagingSession",
	"26":  "ReliableMessagingStream",
	"48":  "P2P",
	"49":  "RoutingTable",
	"50":  "Token",
	"51":  "VoteProxy",
	"52":  "VoteManager",
	"53":  "Bootstrap",
	"54":  "Join",
	"55":  "JoinLock",
	"56":  "Gap",
	"57":  "Lease",
	"58":  "Arbitration",
	"59":  "Routing",
	"60":  "Broadcast",
	"61":  "SiteNode",
	"62":  "Multicast",
	"64":  "Reliability",
	"65":  "Replication",
	"66":  "OperationQueue",
	"67":  "Api",
	"68":  "CRM",
	"69":  "MCRM",
	"72":  "FM",
	"73":  "RA",
	"74":  "RAP",
	"75":  "FailoverUnit",
	"76":  "FMM",
	"77":  "PLB",
	"78":  "PLBM",
	"79":  "RocksDbStore",
	"80":  "EseStore",
	"81":  "ReplicatedStore",
	"82":  "ReplicatedStoreStateMachine",
	"83":  "SqlStore",
	"84":  "KeyValueStore",
	"85":  "NM",
	"86":  "NamingStoreService",
	"87":  "NamingGateway",
	"88":  "NamingCommon",
	"89":  "HealthClient",
	"90":  "Hosting",
	"91":  "FabricWorkerProcess",
	"92":  "ClrRuntimeHost",
	"93":  "FabricTypeHost",
	"100": "FabricNode",
	"105": "ClusterManagerTransport",
	"106": "FaultAnalysisService",
	"107": "UpgradeOrchestrationService",
	"108": "BackupRestoreService",
	"110": "ServiceModel",
	"111": "ImageStore",
	"112": "NativeImageStoreClient",
	"115": "ClusterManager",
	"116": "RepairPolicyEngineService",
	"117": "InfrastructureService",
	"118": "RepairService",
	"119": "RepairManager",
	"120": "LeaseLayer",
	"125": "CentralSecretService",
	"126": "LocalSecretService",
	"127": "ResourceManager",
	"130": "ServiceGroupCommon",
	"131": "ServiceGroupStateful",
	"132": "ServiceGroupStateless",
	"133": "ServiceGroupStatefulMember",
	"134": "ServiceGroupStatelessMember",
	"135": "ServiceGroupReplication",
	"170": "BwTree",
	"171": "BwTreeVolatileStorage",
	"172": "BwTreeStableStorage",
	"173": "FabricTransport",
	"174": "TestabilityComponent",
	"189": "RCR",
	"190": "LR",
	"191": "SM",
	"192": "RStore",
	"193": "TR",
	"194": "LogicalLog",
	"195": "Ktl",
	"196": "KtlLoggerNode",
	"200": "TestSession",
	"201": "HttpApplicationGateway",
	"209": "DNS",
	"204": "BA",
	"205": "BAP",
	"208": "ResourceMonitor",
	"210": "FabricSetup",
	"211": "Query",
	"212": "HealthManager",
	"213": "HealthManagerCommon",
	"214": "SystemServices",
	"215": "FabricActivator",
	"216": "HttpGateway",
	"217": "Client",
	"218": "FileStoreService",
	"219": "TokenValidationService",
	"220": "AccessControl",
	"221": "FileTransfer",
	"222": "FabricInstallerService",
	"223": "EntreeServiceProxy",
	"256": "MaxTask",
	// Mappings found directly in the Event Viewer
	"249": "TReplicator",
	"251": "TStateManager",
}
