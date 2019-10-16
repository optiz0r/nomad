package monitor

import (
	"sync"

	log "github.com/hashicorp/go-hclog"
)

type Monitor struct {
	sync.Mutex
	sink         log.MultiSinkLogger
	logger       log.MultiSinkLogger
	logCh        chan []byte
	index        int
	droppedCount int
	bufSize      int
}

func New(buf int, sink log.MultiSinkLogger, opts *log.LoggerOptions) *Monitor {
	sw := &Monitor{
		sink:    sink,
		logCh:   make(chan []byte, buf),
		index:   0,
		bufSize: buf,
	}

	opts.Output = sw
	logger := log.New(opts).(log.MultiSinkLogger)
	sw.logger = logger

	return sw
}

func (d *Monitor) Start(stopCh <-chan struct{}) chan []byte {
	d.sink.RegisterSink(d.logger)

	logCh := make(chan []byte, d.bufSize)
	go func() {
		for {
			select {
			case log := <-d.logCh:
				logCh <- log
			case <-stopCh:
				d.sink.DeregisterSink(d.logger)
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
	d.Lock()
	defer d.Unlock()

	select {
	case d.logCh <- p:
	default:
		d.droppedCount++
		if d.droppedCount > 10 {
			d.logger.Warn("Monitor dropped %d logs during monitor request", d.droppedCount)
			d.droppedCount = 0
		}
	}
	return
}
