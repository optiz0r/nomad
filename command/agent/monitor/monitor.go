package monitor

import (
	"sync"

	log "github.com/hashicorp/go-hclog"
)

type Monitor struct {
	sync.Mutex
	sink         *log.Sink
	logger       log.MultiSinkLogger
	logCh        chan []byte
	index        int
	droppedCount int
	bufSize      int
}

func New(buf int, logger log.MultiSinkLogger, opts log.SinkOptions) *Monitor {
	sw := &Monitor{
		logger:  logger,
		logCh:   make(chan []byte, buf),
		index:   0,
		bufSize: buf,
	}

	sink := log.NewSink(&log.SinkOptions{
		Level:      opts.Level,
		JSONFormat: opts.JSONFormat,
		Output:     sw,
	})

	sw.sink = sink
	return sw
}

func (d *Monitor) Start(stopCh <-chan struct{}) <-chan []byte {
	d.logger.RegisterSink(d.sink)

	logCh := make(chan []byte, d.bufSize)
	go func() {
		for {
			select {
			case log := <-d.logCh:
				logCh <- log
			case <-stopCh:
				d.logger.DeregisterSink(d.sink)
				close(d.logCh)
				return
			}
		}
	}()

	return logCh
}

// Write attemps to send latest log to logCh
// it drops the log if channel is unavailable to receive
func (d *Monitor) Write(p []byte) (n int, err error) {
	bytes := make([]byte, len(p))
	copy(bytes, p)

	select {
	case d.logCh <- bytes:
	default:
		d.Lock()
		defer d.Unlock()
		d.droppedCount++
		if d.droppedCount > 10 {
			d.logger.Warn("Monitor dropped %d logs during monitor request", d.droppedCount)
			d.droppedCount = 0
		}
	}
	return
}
