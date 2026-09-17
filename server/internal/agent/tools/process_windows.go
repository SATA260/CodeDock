//go:build windows

package tools

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
}

func killProcessTree(process *os.Process) error {
	command := exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F")
	if err := command.Run(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		return process.Kill()
	}
	return nil
}
