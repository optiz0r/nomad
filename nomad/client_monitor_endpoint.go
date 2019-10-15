package nomad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	log "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/acl"
	cstructs "github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/command/agent/monitor"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/nomad/structs"

	"github.com/ugorji/go/codec"
)

type Monitor struct {
	srv *Server
}

func (m *Monitor) register() {
	m.srv.streamingRpcs.Register("Agent.Monitor", m.monitor)
}

func (m *Monitor) monitor(conn io.ReadWriteCloser) {
	defer conn.Close()

	// Decode args
	var args cstructs.MonitorRequest
	decoder := codec.NewDecoder(conn, structs.MsgpackHandle)
	encoder := codec.NewEncoder(conn, structs.MsgpackHandle)

	if err := decoder.Decode(&args); err != nil {
		handleStreamResultError(err, helper.Int64ToPtr(500), encoder)
		return
	}

	// Check node read permissions
	if aclObj, err := m.srv.ResolveToken(args.AuthToken); err != nil {
		handleStreamResultError(err, nil, encoder)
		return
	} else if aclObj != nil && !aclObj.AllowNsOp(args.Namespace, acl.NamespaceCapabilityReadFS) {
		handleStreamResultError(structs.ErrPermissionDenied, nil, encoder)
		return
	}

	var logLevel log.Level
	if args.LogLevel == "" {
		logLevel = log.LevelFromString("INFO")
	} else {
		logLevel = log.LevelFromString(args.LogLevel)
	}

	if logLevel == log.NoLevel {
		handleStreamResultError(errors.New("Unknown log level"), helper.Int64ToPtr(400), encoder)
		return
	}

	if args.NodeID != "" {
		nodeID := args.NodeID

		snap, err := m.srv.State().Snapshot()
		if err != nil {
			handleStreamResultError(err, nil, encoder)
			return
		}

		node, err := snap.NodeByID(nil, nodeID)
		if err != nil {
			handleStreamResultError(err, helper.Int64ToPtr(500), encoder)
			return
		}

		if node == nil {
			err := fmt.Errorf("Unknown node %q", nodeID)
			handleStreamResultError(err, helper.Int64ToPtr(400), encoder)
			return
		}

		if err := nodeSupportsRpc(node); err != nil {
			handleStreamResultError(err, helper.Int64ToPtr(400), encoder)
			return
		}

		// Get the Connection to the client either by fowarding to another server
		// or creating direct stream
		var clientConn net.Conn
		state, ok := m.srv.getNodeConn(nodeID)
		if !ok {
			// Determine the server that has a connection to the node
			srv, err := m.srv.serverWithNodeConn(nodeID, m.srv.Region())
			if err != nil {
				var code *int64
				if structs.IsErrNoNodeConn(err) {
					code = helper.Int64ToPtr(404)
				}
				handleStreamResultError(err, code, encoder)
				return
			}
			conn, err := m.srv.streamingRpc(srv, "Agent.Monitor")
			if err != nil {
				handleStreamResultError(err, nil, encoder)
				return
			}

			clientConn = conn
		} else {
			stream, err := NodeStreamingRpc(state.Session, "Agent.Monitor")
			if err != nil {
				handleStreamResultError(err, nil, encoder)
				return
			}
			clientConn = stream
		}
		defer clientConn.Close()

		// Send the Request
		outEncoder := codec.NewEncoder(clientConn, structs.MsgpackHandle)
		if err := outEncoder.Encode(args); err != nil {
			handleStreamResultError(err, nil, encoder)
			return
		}

		structs.Bridge(conn, clientConn)
		return
	} else {

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		monitor := monitor.NewStreamWriter(512, m.srv.logger, &log.LoggerOptions{
			Level:      logLevel,
			JSONFormat: false,
		})

		monitor.Monitor(ctx, cancel, conn, encoder, decoder)
	}
}
