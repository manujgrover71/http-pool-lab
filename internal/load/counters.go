package load

import (
	"sync/atomic"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/csvw"
	"github.com/manujgrover71/http-pool-lab/internal/stat"
)

var connColumns = []string{
	"t", "opened", "closed", "live", "dial_errors", "got_reused", "got_new", "handshakes",
	"inflight", "done", "failed", "timewait",
}

type counters struct {
	inflight       atomic.Int64
	opened, closed atomic.Int64 // cumulative

	// reset every tick
	dialErr, gotReused, gotNew, handshakes, done, failed atomic.Int64
}

func tick(w *csvw.File, c *counters, start time.Time, stop <-chan struct{}) error {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var lastOpened, lastClosed int64
	for {
		select {
		case now := <-t.C:
			opened, closed := c.opened.Load(), c.closed.Load()
			w.Write(
				int(now.Sub(start).Seconds()), opened-lastOpened, closed-lastClosed, opened-closed,
				c.dialErr.Swap(0), c.gotReused.Swap(0), c.gotNew.Swap(0), c.handshakes.Swap(0),
				c.inflight.Load(), c.done.Swap(0), c.failed.Swap(0), stat.TimeWait(),
			)
			lastOpened, lastClosed = opened, closed
		case <-stop:
			return w.Close()
		}
	}
}
