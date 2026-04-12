package cron

import (
	"time"

	"github.com/TsekNet/converge/extensions"
)

// PollInterval returns the polling interval for cron/scheduled task state.
func (c *Cron) PollInterval() time.Duration {
	return 5 * time.Minute
}

var _ extensions.Poller = (*Cron)(nil)
