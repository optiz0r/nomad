package monitor

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestMonitor_Start(t *testing.T) {
	t.Parallel()

	logger := log.NewMultiSink(&log.LoggerOptions{
		Level: log.Error,
	})

	m := New(512, logger, log.SinkOptions{
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

func TestMonitor_Write(t *testing.T) {
	t.Parallel()

	_ = []struct {
		desc         string
		receivable   bool
		droppedCount int
	}{
		{
			desc:         "no receiving chan results in dropped log",
			droppedCount: 1,
		},
		{
			desc:         "does not drop messages when receiver is ready",
			receivable:   true,
			droppedCount: 0,
		},
	}

	logger := log.NewMultiSink(&log.LoggerOptions{
		Level: log.Warn,
	})

	m := New(512, logger, log.SinkOptions{
		Level: log.Debug,
	})

	doneCh := make(chan struct{})
	defer close(doneCh)

	logCh := m.Start(doneCh)

	logger.Debug("log 1")
	logger.Debug("log 2")
	logger.Debug("log 3")

	var logs bytes.Buffer
	fmt.Println(string(<-logCh))
	fmt.Println(string(<-logCh))
	fmt.Println(string(<-logCh))

	time.Sleep(5 * time.Second)
	assert.Contains(t, logs, "log 3")
	// spew.Dump(logs.String())
	// assert.Equal(t, tc.droppedCount, m.droppedCount)
	// for _, tc := range cases {
	// 	t.Run(tc.desc, func(t *testing.T) {
	// 		if tc.receivable {

	// 		} else {

	// logger.Debug("log 1")
	// logger.Debug("log 2")
	// logger.Debug("log 3")

	// var logs bytes.Buffer
	// fmt.Println(string(<-logCh))
	// fmt.Println(string(<-logCh))
	// fmt.Println(string(<-logCh))

	// time.Sleep(5 * time.Second)
	// assert.Contains(t, logs, "log 3")
	// // spew.Dump(logs.String())
	// assert.Equal(t, tc.droppedCount, m.droppedCount)
	// 	}
	// })
	// }
}
