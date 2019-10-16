package monitor

import (
	"testing"
	"time"

	log "github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestMonitor_Start(t *testing.T) {
	t.Parallel()

	logger := log.New(&log.LoggerOptions{
		Level: log.Error,
	}).(log.MultiSinkLogger)

	m := New(512, logger, &log.LoggerOptions{
		Level: log.Debug,
	})

	closeCh := make(chan struct{})
	defer close(closeCh)

	logCh := m.Start(closeCh)
	go func() {
		for {
			select {
			case log := <-logCh:
				require.Contains(t, string(log), "[DEBUG] test log")
			case <-time.After(1 * time.Second):
				t.Fatal("Expected to receive from log channel")
			}
		}
	}()
	logger.Debug("test log")
}
