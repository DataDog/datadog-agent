package probe

import (
	"C"

	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/model"
)
import "log"

// handleSecurityInodeSetattr - Handles a setattr update event
func (p *Probe) handleSecurityInodeSetattr(data []byte) {
	eventRaw := &model.SetAttrRaw{}
	err := eventRaw.UnmarshalBinary(data)
	if err != nil {
		log.Printf("failed to decode SetAttrRaw event: %s", err)
		return
	}

	event := model.SetAttrEvent{
		EventBase: model.EventBase{
			EventType: model.FileSetAttrEventType,
			Timestamp: p.StartTime.Add(time.Duration(eventRaw.TimestampRaw) * time.Nanosecond),
		},
		SetAttrRaw: eventRaw,
		TTYName:    C.GoString((*C.char)(unsafe.Pointer(&eventRaw.TTYNameRaw))),
		Atime:      time.Unix(eventRaw.AtimeRaw[0], eventRaw.AtimeRaw[1]),
		Mtime:      time.Unix(eventRaw.MtimeRaw[0], eventRaw.MtimeRaw[1]),
		Ctime:      time.Unix(eventRaw.CtimeRaw[0], eventRaw.CtimeRaw[1]),
	}

	event.Pathname, err = p.resolveDentryPath(eventRaw.PathnameKey)
	if err != nil {
		log.Printf("failed to resolve dentry path (setAttr): %s\n", err)
	}

	// p.DispatchEvent(&event)
}
