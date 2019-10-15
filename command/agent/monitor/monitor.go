package monitor

import (
	"context"
	"io"
	"strings"
	"sync"

	log "github.com/hashicorp/go-hclog"
	cstructs "github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/helper"
	"github.com/ugorji/go/codec"
)

type StreamWriter struct {
	sync.Mutex
	sink         log.MultiSinkLogger
	logger       log.MultiSinkLogger
	logs         []string
	logCh        chan []byte
	index        int
	droppedCount int
}

func (d *StreamWriter) Monitor(ctx context.Context, cancel context.CancelFunc,
	conn io.ReadWriteCloser, enc *codec.Encoder, dec *codec.Decoder) {
	d.sink.RegisterSink(d.logger)
	defer d.sink.DeregisterSink(d.logger)

	// detect the remote side closing
	go func() {
		if _, err := conn.Read(nil); err != nil {
			cancel()
			return
		}
		select {
		case <-ctx.Done():
			return
		}
	}()

	var streamErr error
OUTER:
	for {
		select {
		case log := <-d.logCh:

			var resp cstructs.StreamErrWrapper
			resp.Payload = log
			if err := enc.Encode(resp); err != nil {
				streamErr = err
				break OUTER
			}
			enc.Reset(conn)
		case <-ctx.Done():
			break OUTER
		}
	}

	if streamErr != nil {
		// Nothing to do as conn is closed
		if streamErr == io.EOF || strings.Contains(streamErr.Error(), "closed") {
			return
		}

		// Attempt to send the error
		enc.Encode(&cstructs.StreamErrWrapper{
			Error: cstructs.NewRpcError(streamErr, helper.Int64ToPtr(500)),
		})
		return
	}
}

func NewStreamWriter(buf int, sink log.MultiSinkLogger, opts *log.LoggerOptions) *StreamWriter {
	sw := &StreamWriter{
		sink:  sink,
		logs:  make([]string, buf),
		logCh: make(chan []byte, buf),
		index: 0,
	}

	opts.Output = sw
	logger := log.New(opts).(log.MultiSinkLogger)
	sw.logger = logger

	return sw

}

func (d *StreamWriter) Write(p []byte) (n int, err error) {
	d.Lock()
	defer d.Unlock()

	// Strip off newlines at the end if there are any since we store
	// individual log lines in the agent.
	// n = len(p)
	// if p[n-1] == '\n' {
	// 	p = p[:n-1]
	// }

	d.logs[d.index] = string(p)
	d.index = (d.index + 1) % len(d.logs)

	select {
	case d.logCh <- p:
	default:
		d.droppedCount++
	}
	return
}
