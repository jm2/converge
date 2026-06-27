//go:build linux

package condition

import (
	"context"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// mountPollTimeoutMs bounds each poll(2) wait so ctx cancellation is observed
// promptly even when the mount table is idle.
const mountPollTimeoutMs = 500

// Wait blocks until path becomes (or stops being) a mount point, or ctx is done.
//
// procfs files never deliver inotify IN_MODIFY events, so watching
// /proc/self/mountinfo with inotify never fires on mount/unmount. The kernel
// instead signals mount-table changes by making the open mountinfo fd report
// out-of-band data: poll(2) returns POLLPRI|POLLERR whenever the mount table
// changes. The file must be drained to EOF to arm the next notification;
// without that, poll(2) returns immediately in a busy loop. We re-evaluate
// Met on every wake.
func (c *mountPointCondition) Wait(ctx context.Context) error {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4096)
	pfd := []unix.PollFd{{
		Fd:     int32(f.Fd()),
		Events: unix.POLLPRI | unix.POLLERR,
	}}

	for {
		// Drain to EOF to consume the current mount-table generation; this
		// arms poll(2) to fire POLLPRI on the next change.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		for {
			n, err := f.Read(buf)
			if err == io.EOF || (n == 0 && err == nil) {
				break
			}
			if err != nil {
				return err
			}
		}

		if met, _ := c.Met(ctx); met {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// poll(2) wakes on POLLPRI|POLLERR (mount change) or on timeout; either
		// way we loop back, re-drain, and re-evaluate Met.
		if _, err := unix.Poll(pfd, mountPollTimeoutMs); err != nil {
			if err == unix.EINTR {
				continue
			}
			return err
		}
	}
}
