// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsevent

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
)

var testData = [][2]string{
	{
		`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`,
		`{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"7036","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
	// Without <Data></Data>
	{
		`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`,
		`{"Event":{"EventData":{"Binary":"EventLog/1"},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"7036","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
	// Without <Binary></Binary>
	{
		`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`,
		`{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"7036","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
	// With #text in the text field: it should not be replaced
	{
		`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>#text</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`,
		`{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"#text","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
	// With {"#text":"something"} in the text field: it should not be replaced
	{
		`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>{"#text":"something"}</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`,
		`{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"{\"#text\":\"something\"}","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
	// Test an event without EventID Qualifiers attribute
	{
		`<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event"><System><Provider Name="Microsoft-Windows-WindowsUpdateClient" Guid="{945a8954-c147-4acd-923f-40c45405a658}" /><EventID>19</EventID><Version>1</Version><Level>4</Level><Task>1</Task><Opcode>13</Opcode><Keywords>0x8000000000000018</Keywords><TimeCreated SystemTime="2022-09-30T13:44:36.6772228Z" /><EventRecordID>1868</EventRecordID><Correlation /><Execution ProcessID="11216" ThreadID="11400" /><Channel>System</Channel><Computer>DESKTOP-U86BVDJ</Computer><Security UserID="S-1-5-18" /></System><EventData><Data Name="updateTitle">Security Intelligence Update for Microsoft Defender Antivirus - KB2267602 (Version 1.375.1243.0)</Data><Data Name="updateGuid">{23315d09-c6f2-4cb7-8b40-869952c28480}</Data><Data Name="updateRevisionNumber">200</Data><Data Name="serviceGuid">{9482f4b4-e343-43b6-b170-9a65bc822c77}</Data></EventData></Event>`,
		`{"Event":{"EventData":{"Data":{"serviceGuid":"{9482f4b4-e343-43b6-b170-9a65bc822c77}","updateGuid":"{23315d09-c6f2-4cb7-8b40-869952c28480}","updateRevisionNumber":"200","updateTitle":"Security Intelligence Update for Microsoft Defender Antivirus - KB2267602 (Version 1.375.1243.0)"}},"System":{"Channel":"System","Computer":"DESKTOP-U86BVDJ","Correlation":"","EventID":"19","EventRecordID":"1868","Execution":{"ProcessID":"11216","ThreadID":"11400"},"Keywords":"0x8000000000000018","Level":"4","Opcode":"13","Provider":{"Guid":"{945a8954-c147-4acd-923f-40c45405a658}","Name":"Microsoft-Windows-WindowsUpdateClient"},"Security":{"UserID":"S-1-5-18"},"Task":"1","TimeCreated":{"SystemTime":"2022-09-30T13:44:36.6772228Z"},"Version":"1"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`,
	},
}

func TestToMessageStructuredContent(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, &Config{ChannelPath: "System"}, nil)

	tailer.config.V1Behavior = false

	for _, testCase := range testData {
		actual, _ := tailer.toMessage(richEventFromXML(testCase[0]))
		data, err := actual.Render()
		assert.NoError(t, err)
		assert.Equal(t, testCase[1], string(data))
		assert.Equal(t, []byte{}, actual.GetContent()) // this should not be filled anymore
	}
}

func TestToMessage(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, &Config{ChannelPath: "System"}, nil)

	tailer.config.V1Behavior = true

	for _, testCase := range testData {
		actual, _ := tailer.toMessage(richEventFromXML(testCase[0]))
		assert.Equal(t, testCase[1], string(actual.GetContent()))
	}

	// Test value map render (e.g. level:4 -> level:Warning)
	evt := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":"7036","EventIDQualifier":"16384","EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"OpCode","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"taskName","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"},"level":"Warning","message":"Some message"}`
	richEvt := &richEvent{
		xmlEvent: evt,
		message:  "Some message",
		task:     "taskName",
		opcode:   "OpCode",
		level:    "Warning",
	}
	actual, _ := tailer.toMessage(richEvt)
	assert.Equal(t, expected, string(actual.GetContent()))
}

func richEventFromXML(xml string) *richEvent {
	return &richEvent{xmlEvent: xml}
}

func TestTailerCompareUnstructuredAndStructured(t *testing.T) {
	assert := assert.New(t)
	sourceV1 := sources.NewLogSource("", &config.LogsConfig{})
	tailerV1 := NewTailer(sourceV1, &Config{ChannelPath: "System"}, nil)
	tailerV1.config.V1Behavior = true

	sourceV2 := sources.NewLogSource("", &config.LogsConfig{})
	tailerV2 := NewTailer(sourceV2, &Config{ChannelPath: "System"}, nil)
	tailerV2.config.V1Behavior = false

	for _, testCase := range testData {
		ev1 := &richEvent{
			xmlEvent: testCase[0],
			message:  "some content in the message",
			task:     "rdTaskName",
			opcode:   "OpCode",
			level:    "Warning",
		}
		ev2 := &richEvent{
			xmlEvent: testCase[0],
			message:  "some content in the message",
			task:     "rdTaskName",
			opcode:   "OpCode",
			level:    "Warning",
		}

		messagev1, err1 := tailerV1.toMessage(ev1)
		messagev2, err2 := tailerV2.toMessage(ev2)

		assert.NoError(err1)
		assert.NoError(err2)

		rendered1, err1 := messagev1.Render()
		rendered2, err2 := messagev2.Render()

		assert.NoError(err1)
		assert.NoError(err2)

		assert.Equal(rendered1, rendered2)
	}
}
