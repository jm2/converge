//go:build windows

package hostname

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/TsekNet/converge/extensions"
)

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procSetComputerNameExW = kernel32.NewProc("SetComputerNameExW")
)

// Apply sets the Windows hostname via SetComputerNameExW.
// A reboot is required for the change to take effect.
func (h *Hostname) Apply(_ context.Context) (*extensions.Result, error) {
	match, err := h.alreadySet()
	if err != nil {
		return nil, err
	}
	if match {
		return &extensions.Result{Changed: false, Status: extensions.StatusOK, Message: "already set"}, nil
	}

	namePtr, err := windows.UTF16PtrFromString(h.Name)
	if err != nil {
		return nil, fmt.Errorf("encode hostname %q: %w", h.Name, err)
	}

	ret, _, callErr := procSetComputerNameExW.Call(
		windows.ComputerNamePhysicalDnsHostname,
		uintptr(unsafe.Pointer(namePtr)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("SetComputerNameExW(%s): %w", h.Name, callErr)
	}

	return &extensions.Result{Changed: true, Status: extensions.StatusChanged, Message: "set (reboot required)"}, nil
}
