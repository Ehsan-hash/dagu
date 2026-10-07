// Copyright (C) 2026 Yota Hamada
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// startDetached starts the command in its own session, so the caller's
// terminal or process group ending does not end it.
func startDetached(command string, args []string) error {
	cmd := exec.Command(command, args...) //nolint:gosec // starting this executable's own watchdog is the purpose
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
