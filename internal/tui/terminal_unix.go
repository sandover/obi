//go:build darwin || linux

package tui

import (
	"fmt"
	"syscall"
	"unsafe"
)

type termState struct {
	termios syscall.Termios
}

type systemTerminal struct{}

func (systemTerminal) makeRaw(fd int) (*termState, error) {
	var state termState
	if err := ioctl(fd, ioctlReadTermios, unsafe.Pointer(&state.termios)); err != nil {
		return nil, fmt.Errorf("tcgetattr: %w", err)
	}
	raw := state.termios
	cfmakeraw(&raw)
	if err := ioctl(fd, ioctlWriteTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, fmt.Errorf("tcsetattr: %w", err)
	}
	return &state, nil
}

func (systemTerminal) restore(fd int, state *termState) error {
	if state == nil {
		return nil
	}
	if err := ioctl(fd, ioctlWriteTermios, unsafe.Pointer(&state.termios)); err != nil {
		return fmt.Errorf("restore termios: %w", err)
	}
	return nil
}

func (systemTerminal) getSize(fd int) (int, int, error) {
	var ws winsize
	if err := ioctl(fd, ioctlGetWinsize, unsafe.Pointer(&ws)); err != nil {
		return 0, 0, fmt.Errorf("get winsize: %w", err)
	}
	return int(ws.Col), int(ws.Row), nil
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func ioctl(fd int, req uintptr, data unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(data))
	if errno != 0 {
		return errno
	}
	return nil
}

func cfmakeraw(termios *syscall.Termios) {
	termios.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	termios.Oflag &^= syscall.OPOST
	termios.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	termios.Cflag &^= syscall.CSIZE | syscall.PARENB
	termios.Cflag |= syscall.CS8
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0
}
