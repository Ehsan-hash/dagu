// Copyright (C) 2026 Yota Hamada
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cmd

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startDetached starts the command without a console, outside the caller's
// job object when the job allows it, so closing the job does not end it.
func startDetached(command string, args []string) error {
	err := startWithFlags(command, args, windows.CREATE_NEW_PROCESS_GROUP|windows.DETACHED_PROCESS|windows.CREATE_BREAKAWAY_FROM_JOB)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// The job forbids breaking away; start the command inside it.
		err = startWithFlags(command, args, windows.CREATE_NEW_PROCESS_GROUP|windows.DETACHED_PROCESS)
	}
	return err
}

func startWithFlags(command string, args []string, flags uint32) error {
	cmd := exec.Command(command, args...) //nolint:gosec // starting this executable's own watchdog is the purpose
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
