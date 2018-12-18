// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package windowsevent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToMessage(t *testing.T) {
	tailer := NewTailer(nil, &Config{ChannelPath: "System"}, nil)
	evt1 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected1 := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ := tailer.toMessage(evt1)
	assert.Equal(t, expected1, string(actual.Content))

	// Without <Data></Data>
	evt2 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected2 := `{"Event":{"EventData":{"Binary":"EventLog/1"},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt2)
	assert.Equal(t, expected2, string(actual.Content))

	// Without <Binary></Binary>
	evt3 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`
	expected3 := `{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt3)
	assert.Equal(t, expected3, string(actual.Content))

	// With #text in the text field: it should not be replaced
	evt4 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>#text</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`
	expected4 := `{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"#text","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt4)
	assert.Equal(t, expected4, string(actual.Content))

	// With {"#text":"something"} in the text field: it should not be replaced
	evt5 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>{"#text":"something"}</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data></EventData></Event>`
	expected5 := `{"Event":{"EventData":{"Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"{\"#text\":\"something\"}","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt5)
	assert.Equal(t, expected5, string(actual.Content))
}

func TestToMessageWithFabric(t *testing.T) {
	tailer := NewTailer(nil, &Config{ChannelPath: "Microsoft-ServiceFabric/Operational"}, nil)

	// With <Task>0</Task>: doesn't exist in the mapping, should not change
	evt1 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>0</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected1 := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"0","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ := tailer.toMessage(evt1)
	assert.Equal(t, expected1, string(actual.Content))

	// With <Task>1</Task>: should be mapped to "Common"
	evt2 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Task>1</Task><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected2 := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","Task":"Common","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt2)
	assert.Equal(t, expected2, string(actual.Content))

	// Without <Task></Task> field
	evt3 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/><EventID Qualifiers='16384'>7036</EventID><Version>0</Version><Level>4</Level><Opcode>0</Opcode><Keywords>0x8080000000000000</Keywords><TimeCreated SystemTime='2013-08-22T14:51:44.205667300Z'/><EventRecordID>2</EventRecordID><Correlation/><Execution ProcessID='516' ThreadID='1792'/><Channel>System</Channel><Computer>windows-n7iefg2</Computer><Security/></System><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected3 := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"System":{"Channel":"System","Computer":"windows-n7iefg2","Correlation":"","EventID":{"value":"7036","Qualifiers":"16384"},"EventRecordID":"2","Execution":{"ProcessID":"516","ThreadID":"1792"},"Keywords":"0x8080000000000000","Level":"4","Opcode":"0","Provider":{"EventSourceName":"Service Control Manager","Guid":"{555908d1-a6d7-4695-8e1e-26931d2012f4}","Name":"Service Control Manager"},"Security":"","TimeCreated":{"SystemTime":"2013-08-22T14:51:44.205667300Z"},"Version":"0"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt3)
	assert.Equal(t, expected3, string(actual.Content))

	// Without <System></System> field
	evt4 := `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><EventData><Data Name='param1'>Windows Event Log</Data><Data Name='param2'>stopped</Data><Binary>4500760065006E0074004C006F0067002F0031000000</Binary></EventData></Event>`
	expected4 := `{"Event":{"EventData":{"Binary":"EventLog/1","Data":{"param1":"Windows Event Log","param2":"stopped"}},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt4)
	assert.Equal(t, expected4, string(actual.Content))

	// More complex event
	evt5 := `<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event"><System><Provider Name="Microsoft-ServiceFabric" Guid="{CBD93BC2-71E5-4566-B3A7-595D8EECA6E8}" /><EventID>18545</EventID><Version>1</Version><Level>3</Level><Task>72</Task><Opcode>0</Opcode><Keywords>0x8000000000000001</Keywords><TimeCreated SystemTime="2018-11-27T09:49:31.206062100Z" /><EventRecordID>35563</EventRecordID><Correlation /><Execution ProcessID="2144" ThreadID="2912" /><Channel>Microsoft-ServiceFabric/Admin</Channel><Computer>default000000</Computer><Security UserID="S-1-5-20" /></System><EventData><Data Name="id">{00000000-0000-0000-0000-000000005000}</Data><Data Name="replicaId">131877851854953235</Data><Data Name="nodeId._type:LargeInteger_high">0x19e07e4c94e75af5</Data><Data Name="nodeId.low">0xa4be977c55bd4164</Data><Data Name="error.val">2147942512</Data></EventData></Event>`
	expected5 := `{"Event":{"EventData":{"Data":{"error.val":"2147942512","id":"{00000000-0000-0000-0000-000000005000}","nodeId._type:LargeInteger_high":"0x19e07e4c94e75af5","nodeId.low":"0xa4be977c55bd4164","replicaId":"131877851854953235"}},"System":{"Channel":"Microsoft-ServiceFabric/Admin","Computer":"default000000","Correlation":"","EventID":"18545","EventRecordID":"35563","Execution":{"ProcessID":"2144","ThreadID":"2912"},"Keywords":"0x8000000000000001","Level":"3","Opcode":"0","Provider":{"Guid":"{CBD93BC2-71E5-4566-B3A7-595D8EECA6E8}","Name":"Microsoft-ServiceFabric"},"Security":{"UserID":"S-1-5-20"},"Task":"FM","TimeCreated":{"SystemTime":"2018-11-27T09:49:31.206062100Z"},"Version":"1"},"xmlns":"http://schemas.microsoft.com/win/2004/08/events/event"}}`
	actual, _ = tailer.toMessage(evt5)
	assert.Equal(t, expected5, string(actual.Content))
}
