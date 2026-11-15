//go:build !windows

package host

import (
	"context"
	"errors"
)

var errWindowsOnly = errors.New("此操作仅适用于 Windows")

func windowsVirtualization(context.Context) VirtualizationReport { return readyReport() }

func EnableWindowsVirtualization(context.Context) error { return errWindowsOnly }

func RestartIntoFirmware(context.Context) error { return errWindowsOnly }

func RestartWindows(context.Context) error { return errWindowsOnly }
