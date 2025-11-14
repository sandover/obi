//go:build !darwin && !linux

package tui

import "fmt"

type termState struct{}

type systemTerminal struct{}

func (systemTerminal) makeRaw(int) (*termState, error) {
	return nil, fmt.Errorf("raw terminal mode unsupported on this platform")
}

func (systemTerminal) restore(int, *termState) error {
	return nil
}

func (systemTerminal) getSize(int) (int, int, error) {
	return 0, 0, fmt.Errorf("terminal sizing unsupported on this platform")
}
