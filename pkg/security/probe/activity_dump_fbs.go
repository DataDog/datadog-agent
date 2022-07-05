// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/adfb"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	flatbuffers "github.com/google/flatbuffers/go"
)

func activityDumpToFBS(ad *ActivityDump) []byte {
	if ad == nil {
		return nil
	}

	b := flatbuffers.NewBuilder(0)

	hostIndex := b.CreateString(ad.Host)
	serviceIndex := b.CreateString(ad.Service)
	sourceIndex := b.CreateString(ad.Source)
	tagsIndices := prepareArrayToFBS(b, len(ad.Tags), func(b *flatbuffers.Builder, i int) flatbuffers.UOffsetT {
		return b.CreateString(ad.Tags[i])
	})
	tagsIndex := arrayToFBS(b, tagsIndices, adfb.ActivityDumpStartTagsVector)
	panIndices := prepareArrayToFBS(b, len(ad.ProcessActivityTree), func(b *flatbuffers.Builder, i int) flatbuffers.UOffsetT {
		return processActivityNodeToFBS(b, ad.ProcessActivityTree[i])
	})
	panIndex := arrayToFBS(b, panIndices, adfb.ActivityDumpStartTreeVector)

	adfb.ActivityDumpStart(b)
	adfb.ActivityDumpAddHost(b, hostIndex)
	adfb.ActivityDumpAddService(b, serviceIndex)
	adfb.ActivityDumpAddSource(b, sourceIndex)
	adfb.ActivityDumpAddTags(b, tagsIndex)
	adfb.ActivityDumpAddTree(b, panIndex)
	adIndex := adfb.ActivityDumpEnd(b)

	b.Finish(adIndex)

	return b.FinishedBytes()
}

func processActivityNodeToFBS(b *flatbuffers.Builder, pan *ProcessActivityNode) flatbuffers.UOffsetT {
	if pan == nil {
		return flatbuffers.UOffsetT(0)
	}

	processNodeIndex := processNodeToFBS(b, &pan.Process)
	generationTypeIndex := b.CreateString(string(pan.GenerationType))
	childrenIndices := prepareArrayToFBS(b, len(pan.Children), func(b *flatbuffers.Builder, i int) flatbuffers.UOffsetT {
		return processActivityNodeToFBS(b, pan.Children[i])
	})
	childrenIndex := arrayToFBS(b, childrenIndices, adfb.ProcessActivityNodeStartChildrenVector)
	filesIndices := make([]flatbuffers.UOffsetT, 0, len(pan.Files))
	for _, c := range pan.Files {
		filesIndices = append(filesIndices, fileActivityNodeToFBS(b, c))
	}
	filesIndex := arrayToFBS(b, filesIndices, adfb.ProcessActivityNodeStartFilesVector)

	adfb.ProcessActivityNodeStart(b)
	adfb.ProcessActivityNodeAddProcess(b, processNodeIndex)
	adfb.ProcessActivityNodeAddGenerationType(b, generationTypeIndex)
	adfb.ProcessActivityNodeAddChildren(b, childrenIndex)
	adfb.ProcessActivityNodeAddFiles(b, filesIndex)
	return adfb.ProcessActivityNodeEnd(b)
}

func processNodeToFBS(b *flatbuffers.Builder, p *model.Process) flatbuffers.UOffsetT {
	if p == nil {
		return flatbuffers.UOffsetT(0)
	}

	containerIdIndex := b.CreateString(p.ContainerID)
	fileEventIndex := fileEventToFBS(b, &p.FileEvent)
	ttyNameIndex := b.CreateString(p.TTYName)
	commIndex := b.CreateString(p.Comm)
	credentialsIndex := credentialsToFBS(b, &p.Credentials)

	argsIndices := prepareArrayToFBS(b, len(p.ScrubbedArgv), func(b *flatbuffers.Builder, i int) flatbuffers.UOffsetT {
		return b.CreateString(p.ScrubbedArgv[i])
	})
	argsIndex := arrayToFBS(b, argsIndices, adfb.ProcessInfoStartArgsVector)
	argv0Index := b.CreateString(p.Argv0)
	envsIndices := prepareArrayToFBS(b, len(p.Envs), func(b *flatbuffers.Builder, i int) flatbuffers.UOffsetT {
		return b.CreateString(p.Envs[i])
	})
	envsIndex := arrayToFBS(b, envsIndices, adfb.ProcessInfoStartEnvsVector)

	adfb.ProcessInfoStart(b)
	adfb.ProcessInfoAddPid(b, p.Pid)
	adfb.ProcessInfoAddTid(b, p.Tid)
	adfb.ProcessInfoAddPpid(b, p.PPid)
	adfb.ProcessInfoAddCookie(b, p.Cookie)
	adfb.ProcessInfoAddIsThread(b, p.IsThread)

	adfb.ProcessInfoAddFile(b, fileEventIndex)

	adfb.ProcessInfoAddContainerId(b, containerIdIndex)
	adfb.ProcessInfoAddSpanId(b, p.SpanID)
	adfb.ProcessInfoAddTraceId(b, p.TraceID)
	adfb.ProcessInfoAddTty(b, ttyNameIndex)
	adfb.ProcessInfoAddComm(b, commIndex)

	adfb.ProcessInfoAddForkTime(b, timestamp(&p.ForkTime))
	adfb.ProcessInfoAddExitTime(b, timestamp(&p.ExitTime))
	adfb.ProcessInfoAddExecTime(b, timestamp(&p.ExecTime))

	adfb.ProcessInfoAddCredentials(b, credentialsIndex)

	adfb.ProcessInfoAddArgs(b, argsIndex)
	adfb.ProcessInfoAddArgv0(b, argv0Index)
	adfb.ProcessInfoAddArgsTruncated(b, p.ArgsTruncated)
	adfb.ProcessInfoAddEnvs(b, envsIndex)
	adfb.ProcessInfoAddEnvsTruncated(b, p.EnvsTruncated)
	return adfb.ProcessInfoEnd(b)
}

func fileEventToFBS(b *flatbuffers.Builder, f *model.FileEvent) flatbuffers.UOffsetT {
	if f == nil {
		return flatbuffers.UOffsetT(0)
	}

	userIndex := b.CreateString(f.User)
	groupIndex := b.CreateString(f.Group)
	pathIndex := b.CreateString(f.PathnameStr)
	basenameIndex := b.CreateString(f.BasenameStr)
	fsIndex := b.CreateString(f.Filesystem)

	adfb.FileInfoStart(b)
	adfb.FileInfoAddUid(b, f.UID)
	adfb.FileInfoAddUser(b, userIndex)
	adfb.FileInfoAddGid(b, f.GID)
	adfb.FileInfoAddGroup(b, groupIndex)
	adfb.FileInfoAddMode(b, f.Mode)
	adfb.FileInfoAddCtime(b, f.CTime)
	adfb.FileInfoAddMtime(b, f.MTime)
	adfb.FileInfoAddMountId(b, f.MountID)
	adfb.FileInfoAddInode(b, f.Inode)
	adfb.FileInfoAddInUpperLayer(b, f.InUpperLayer)
	adfb.FileInfoAddPath(b, pathIndex)
	adfb.FileInfoAddBasename(b, basenameIndex)
	adfb.FileInfoAddFilesystem(b, fsIndex)
	return adfb.FileInfoEnd(b)
}

func credentialsToFBS(b *flatbuffers.Builder, creds *model.Credentials) flatbuffers.UOffsetT {
	if creds == nil {
		return flatbuffers.UOffsetT(0)
	}

	userIndex := b.CreateString(creds.User)
	eUserIndex := b.CreateString(creds.EUser)
	fsUserIndex := b.CreateString(creds.FSUser)
	groupIndex := b.CreateString(creds.Group)
	eGroupIndex := b.CreateString(creds.EGroup)
	fsGroupIndex := b.CreateString(creds.FSGroup)

	adfb.CredentialsStart(b)
	adfb.CredentialsAddUid(b, creds.UID)
	adfb.CredentialsAddGid(b, creds.GID)
	adfb.CredentialsAddUser(b, userIndex)
	adfb.CredentialsAddGroup(b, groupIndex)
	adfb.CredentialsAddEffectiveUid(b, creds.EUID)
	adfb.CredentialsAddEffectiveGid(b, creds.EGID)
	adfb.CredentialsAddEffectiveUser(b, eUserIndex)
	adfb.CredentialsAddEffectiveGroup(b, eGroupIndex)
	adfb.CredentialsAddFsUid(b, creds.FSUID)
	adfb.CredentialsAddFsGid(b, creds.FSGID)
	adfb.CredentialsAddFsUser(b, fsUserIndex)
	adfb.CredentialsAddFsGroup(b, fsGroupIndex)
	adfb.CredentialsAddCapEffective(b, creds.CapEffective)
	adfb.CredentialsAddCapPermitted(b, creds.CapPermitted)
	return adfb.CredentialsEnd(b)
}

func fileActivityNodeToFBS(b *flatbuffers.Builder, fan *FileActivityNode) flatbuffers.UOffsetT {
	if fan == nil {
		return flatbuffers.UOffsetT(0)
	}

	nameIndex := b.CreateString(fan.Name)
	fileEventIndex := fileEventToFBS(b, fan.File)
	genTypeIndex := b.CreateString(string(fan.GenerationType))
	openNodeIndex := openNodeToFBS(b, fan.Open)

	// Children
	childrenIndices := make([]flatbuffers.UOffsetT, 0, len(fan.Children))
	for _, c := range fan.Children {
		childrenIndices = append(childrenIndices, fileActivityNodeToFBS(b, c))
	}
	adfb.FileActivityNodeStartChildrenVector(b, len(childrenIndices))
	for i := len(childrenIndices) - 1; i >= 0; i-- {
		b.PrependUOffsetT(childrenIndices[i])
	}
	childrenIndex := b.EndVector(len(childrenIndices))

	adfb.FileActivityNodeStart(b)
	adfb.FileActivityNodeAddName(b, nameIndex)
	adfb.FileActivityNodeAddFile(b, fileEventIndex)
	adfb.FileActivityNodeAddGenerationType(b, genTypeIndex)
	adfb.FileActivityNodeAddFirstSeen(b, timestamp(&fan.FirstSeen))
	adfb.FileActivityNodeAddOpen(b, openNodeIndex)
	adfb.FileActivityNodeAddChildren(b, childrenIndex)
	return adfb.FileActivityNodeEnd(b)
}

func openNodeToFBS(b *flatbuffers.Builder, open *OpenNode) flatbuffers.UOffsetT {
	if open == nil {
		return flatbuffers.UOffsetT(0)
	}

	adfb.OpenNodeStart(b)

	adfb.OpenNodeAddRetval(b, open.Retval)
	adfb.OpenNodeAddFlags(b, open.Flags)
	adfb.OpenNodeAddMode(b, open.Mode)

	return adfb.OpenNodeEnd(b)
}

func prepareArrayToFBS(b *flatbuffers.Builder, numElems int, elemFn func(*flatbuffers.Builder, int) flatbuffers.UOffsetT) []flatbuffers.UOffsetT {
	indices := make([]flatbuffers.UOffsetT, 0, numElems)
	for i := 0; i < numElems; i++ {
		indices = append(indices, elemFn(b, i))
	}
	return indices
}

func arrayToFBS(b *flatbuffers.Builder, indices []flatbuffers.UOffsetT, startFn func(*flatbuffers.Builder, int) flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	startFn(b, len(indices))
	for i := len(indices) - 1; i >= 0; i-- {
		b.PrependUOffsetT(indices[i])
	}
	return b.EndVector(len(indices))
}
