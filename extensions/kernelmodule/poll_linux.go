//go:build linux

package kernelmodule

import (
	"time"

	"github.com/TsekNet/converge/extensions"
)

// PollInterval returns the polling interval for kernel module state.
// Module changes are infrequent, so a 2-minute interval is appropriate.
func (k *KernelModule) PollInterval() time.Duration {
	return 2 * time.Minute
}

var _ extensions.Poller = (*KernelModule)(nil)
