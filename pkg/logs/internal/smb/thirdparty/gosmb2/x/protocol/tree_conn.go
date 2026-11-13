package protocol

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	pathpkg "github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/path"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/x/wire"
)

type Tree struct {
	*session
	treeId       uint32
	shareType    uint8
	shareFlags   uint32
	capabilities uint32
	isDFSShare   bool
	serverName   string
	shareName    string

	// capabilities uint32
	// maximalAccess uint32
}

var errInvalidTree = errors.New("protocol: invalid tree connection")

func (s *session) treeConnect(ctx context.Context, serverName string, shareName string, flags uint16) (*Tree, error) {
	req := &wire.TreeConnectRequest{
		Flags: flags,
		Path:  pathpkg.JoinUNC(serverName, shareName),
	}

	encrypt := s.sessionFlags&wire.SMB2_SESSION_FLAG_ENCRYPT_DATA != 0
	rrs, err := s.send(ctx, encrypt, req)
	if err != nil {
		return nil, err
	}
	// A server may finish TREE_CONNECT successfully after CANCEL. Retain
	// its final response so the newly created tree can be disconnected.
	rrs[0].waitFinal = true
	res, err := recvAll(rrs, s)
	if err != nil {
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		return nil, err
	}
	defer res.close()

	r := wire.TreeConnectResponseDecoder(res.data(0))
	if r.IsInvalid() {
		return nil, invalidResponse(wire.SMB2_TREE_CONNECT, "broken tree connect response format")
	}

	tc := &Tree{
		session:      s,
		treeId:       res.packet(0).codec().TreeId(),
		shareType:    r.ShareType(),
		shareFlags:   r.ShareFlags(),
		capabilities: r.Capabilities(),
		isDFSShare:   r.Capabilities()&wire.SMB2_SHARE_CAP_DFS != 0,
		serverName:   serverName,
		shareName:    shareName,
		// maximalAccess: r.MaximalAccess(),
	}

	if err := ctx.Err(); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return nil, errors.Join(err, tc.disconnect(cleanupCtx))
	}
	return tc, nil
}

func (tc *Tree) disconnect(ctx context.Context) error {
	req := new(wire.TreeDisconnectRequest)

	res, err := tc.sendRecv(ctx, req)
	if err != nil {
		return err
	}
	defer res.close()

	return nil
}

// Disconnect tears down this tree connection.
func (tc *Tree) Disconnect(ctx context.Context) error {
	if ctx == nil {
		panic("nil context")
	}
	if tc == nil || tc.session == nil || tc.conn == nil {
		return errInvalidTree
	}
	return tc.disconnect(ctx)
}

func (tc *Tree) closeFile(ctx context.Context, fd wire.FileId) error {
	res, err := tc.Request().WithFileID(fd).Close().Do(ctx)
	if err != nil {
		return err
	}
	res.close()

	return nil
}

// CloseFile closes a file handle on this tree.
func (tc *Tree) CloseFile(ctx context.Context, fd wire.FileId) error {
	if ctx == nil {
		panic("nil context")
	}
	if tc == nil || tc.session == nil || tc.conn == nil {
		return errInvalidTree
	}
	return tc.closeFile(ctx, fd)
}

func (tc *Tree) MaxReadSize(companions int) int {
	if tc == nil || tc.session == nil || tc.session.conn == nil {
		return 0
	}
	return tc.effectivePayloadSize(tc.maxReadSize, companions)
}

func (tc *Tree) MaxWriteSize(companions int) int {
	if tc == nil || tc.session == nil || tc.session.conn == nil {
		return 0
	}
	return tc.effectivePayloadSize(tc.maxWriteSize, companions)
}

func (tc *Tree) MaxTransactSize(companions int) int {
	if tc == nil || tc.session == nil || tc.session.conn == nil {
		return 0
	}
	return tc.effectivePayloadSize(tc.maxTransactSize, companions)
}

func (tc *Tree) IOPipelineDepth() uint {
	if tc == nil || tc.session == nil || tc.session.conn == nil {
		return 0
	}
	depth := tc.ioPipelineDepth
	if depth == 0 {
		return clientIOPipelineDepth
	}
	return depth
}

func (tc *Tree) ShareType() uint8 {
	if tc == nil {
		return 0
	}
	return tc.shareType
}

func (tc *Tree) Dialect() uint16 {
	if tc == nil || tc.session == nil || tc.session.conn == nil {
		return 0
	}
	return uint16(tc.dialect)
}

func (tc *Tree) sendRecv(ctx context.Context, reqs ...wire.Packet) (*Response, error) {
	if len(reqs) == 0 {
		return nil, errors.New("protocol: empty request")
	}

	rrs, err := tc.send(ctx, reqs...)
	if err != nil {
		if err == errCompoundCredits {
			return tc.sendRecvSequential(ctx, reqs)
		}
		return nil, err
	}
	hasCreate := hasCreateRequest(reqs)
	if hasCreate {
		for _, rr := range rrs {
			rr.waitFinal = true
		}
	}
	res, err := recvAll(rrs, tc)
	if res != nil {
		res.tree = tc
	}
	if hasCreate && ctx.Err() != nil {
		// CANCEL can lose to a successful CREATE. Drain every related response
		// before deciding whether the server already executed CLOSE. Keeping
		// ownership here also covers a response buffered before cancellation.
		tc.closeResponseFile(reqs, res)
		res.close()
		return nil, ctx.Err()
	}
	return res, err
}

func hasCreateRequest(reqs []wire.Packet) bool {
	for _, req := range reqs {
		if _, ok := req.(*wire.CreateRequest); ok {
			return true
		}
	}
	return false
}

// closeResponseFile reclaims only handles created by this request. A related
// chain uses the preceding operation's FileId, including after CLOSE; it does
// not fall back to an earlier CREATE when the latest handle has been closed.
// The caller retains the response buffers until this function returns.
func (tc *Tree) closeResponseFile(reqs []wire.Packet, res *Response) {
	if res == nil {
		return
	}
	opened := make(map[wire.FileId]struct{})
	var order []wire.FileId
	var related wire.FileId
	hasRelated := false
	for i, req := range reqs {
		if _, ok := req.(*wire.CreateRequest); ok {
			hasRelated = false
			if res.packet(i) == nil {
				continue
			}
			created := wire.CreateResponseDecoder(res.data(i))
			if created.IsInvalid() {
				continue
			}
			related = created.FileId().Decode()
			hasRelated = true
			if _, exists := opened[related]; !exists {
				order = append(order, related)
			}
			opened[related] = struct{}{}
			continue
		}
		fd, usesFileID := requestFileID(req)
		if !usesFileID {
			hasRelated = false
			continue
		}
		if i == 0 {
			related = fd
			hasRelated = true
		}
		if _, closing := req.(*wire.CloseRequest); closing && res.packet(i) != nil && hasRelated {
			delete(opened, related)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientSessionCloseTimeout)
	defer cancel()
	for _, id := range order {
		if _, exists := opened[id]; exists {
			delete(opened, id)
			_ = tc.closeFile(ctx, id)
		}
	}
}

// requestFileID identifies commands which carry a file identifier. The bool
// reports whether the command carries a file identifier.
func requestFileID(req wire.Packet) (wire.FileId, bool) {
	switch r := req.(type) {
	case *wire.CloseRequest:
		return r.FileId, true
	case *wire.FlushRequest:
		return r.FileId, true
	case *wire.ReadRequest:
		return r.FileId, true
	case *DirectReadRequest:
		return r.FileId, true
	case *wire.WriteRequest:
		return r.FileId, true
	case *wire.LockRequest:
		return r.FileId, true
	case *wire.IoctlRequest:
		return r.FileId, true
	case *wire.QueryDirectoryRequest:
		return r.FileId, true
	case *wire.ChangeNotifyRequest:
		return r.FileId, true
	case *wire.QueryInfoRequest:
		return r.FileId, true
	case *wire.SetInfoRequest:
		return r.FileId, true
	default:
		return wire.FileId{}, false
	}
}

func (tc *Tree) send(ctx context.Context, reqs ...wire.Packet) (rrs []*outstandingRequest, err error) {
	if tc.isDFSShare {
		// DFS CREATE requests carry SMB2_FLAGS_DFS_OPERATIONS and the full path
		// name. Rewrite copies so the caller's request objects keep their
		// share-relative name and flags.
		cloned := false
		for i, req := range reqs {
			cr, ok := req.(*wire.CreateRequest)
			if !ok {
				continue
			}
			if !cloned {
				reqs = slices.Clone(reqs)
				cloned = true
			}
			clone := *cr
			clone.Name = tc.dfsPath(cr.Name)
			clone.SetFlags(cr.HeaderFlags() | wire.SMB2_FLAGS_DFS_OPERATIONS)
			reqs[i] = &clone
		}
	}
	for _, req := range reqs {
		req.SetTreeId(tc.treeId)
	}

	encrypt := (tc.session.sessionFlags&wire.SMB2_SESSION_FLAG_ENCRYPT_DATA != 0) || (tc.shareFlags&wire.SMB2_SHAREFLAG_ENCRYPT_DATA != 0)

	rrs, err = tc.session.send(ctx, encrypt, reqs...)
	if err != nil {
		return nil, err
	}

	return rrs, nil
}

// uncPath returns the public UNC path \\server\share\name for a share-relative
// name.
func (tc *Tree) uncPath(name string) string {
	return pathpkg.JoinUNC(tc.serverName, tc.shareName, name)
}

// dfsPath returns the [MS-SMB2] "full path name" \server\share\name for a
// share-relative name, the form required with SMB2_FLAGS_DFS_OPERATIONS
// ([MS-DFSC] 3.2.4.1).
func (tc *Tree) dfsPath(name string) string {
	return tc.uncPath(name)[1:]
}

func (tc *Tree) recv(rr *outstandingRequest) (rp *recvPacket, err error) {
	rp, err = tc.session.recv(rr)
	if err != nil {
		return nil, err
	}
	if asyncId := rr.asyncId.Load(); asyncId != 0 {
		if rpAsyncId := rp.codec().AsyncId(); rpAsyncId != asyncId {
			rp.close()
			return nil, invalidResponse(rr.cmd, fmt.Sprintf("expected async id: %v, got %v", asyncId, rpAsyncId))
		}
	} else {
		if treeId := rp.codec().TreeId(); treeId != tc.treeId {
			rp.close()
			return nil, invalidResponse(rr.cmd, fmt.Sprintf("expected tree id: %v, got %v", tc.treeId, treeId))
		}
	}
	return rp, err
}

// Request starts a compound request against this tree. Symlink following is
// disabled by default for the low-level API.
// (The implementation lives in request.go; this comment documents the Tree
// entry point next to the other tree lifecycle methods.)
