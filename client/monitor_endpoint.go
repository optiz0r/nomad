package client

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/hashicorp/nomad/acl"
	"github.com/hashicorp/nomad/command/agent/monitor"
	"github.com/hashicorp/nomad/helper"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/ugorji/go/codec"

	metrics "github.com/armon/go-metrics"
	log "github.com/hashicorp/go-hclog"
	cstructs "github.com/hashicorp/nomad/client/structs"
)

type Monitor struct {
	c *Client
}

func NewMonitorEndpoint(c *Client) *Monitor {
	m := &Monitor{c: c}
	m.c.streamingRpcs.Register("Agent.Monitor", m.monitor)
	return m
}

func (m *Monitor) monitor(conn io.ReadWriteCloser) {
	defer metrics.MeasureSince([]string{"client", "monitor", "monitor"}, time.Now())
	defer conn.Close()

	// Decode arguments
	var req cstructs.MonitorRequest
	decoder := codec.NewDecoder(conn, structs.MsgpackHandle)
	encoder := codec.NewEncoder(conn, structs.MsgpackHandle)

	if err := decoder.Decode(&req); err != nil {
		handleStreamResultError(err, helper.Int64ToPtr(500), encoder)
		return
	}

	// Check acl
	if aclObj, err := m.c.ResolveToken(req.QueryOptions.AuthToken); err != nil {
		handleStreamResultError(err, helper.Int64ToPtr(403), encoder)
		return
	} else if aclObj != nil && !aclObj.AllowNsOp(req.Namespace, acl.NamespaceCapabilityReadFS) {
		handleStreamResultError(structs.ErrPermissionDenied, helper.Int64ToPtr(403), encoder)
		return
	}

	var logLevel log.Level
	if req.LogLevel == "" {
		logLevel = log.LevelFromString("INFO")
	} else {
		logLevel = log.LevelFromString(req.LogLevel)
	}

	if logLevel == log.NoLevel {
		handleStreamResultError(errors.New("Unknown log level"), helper.Int64ToPtr(400), encoder)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitor := monitor.NewStreamWriter(512, m.c.logger, &log.LoggerOptions{
		Level:      logLevel,
		JSONFormat: false,
	})

	monitor.Monitor(ctx, cancel, conn, encoder, decoder)

}
