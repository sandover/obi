package app

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

type signalSession interface {
	SoftStop(reason string) error
	Abort() error
}

type signalRelay struct {
	handle       signalSession
	out          io.Writer
	softStopSent bool
}

func newSignalRelay(handle signalSession, out io.Writer) *signalRelay {
	if out == nil {
		out = io.Discard
	}
	return &signalRelay{handle: handle, out: out}
}

func (r *signalRelay) handleSignal(sig os.Signal) {
	if r == nil || r.handle == nil {
		return
	}
	switch sig {
	case os.Interrupt:
		if !r.softStopSent {
			r.softStopSent = true
			fmt.Fprintln(r.out, "\nCtrl+C received – requesting soft stop...")
			if err := r.handle.SoftStop("Operator pressed Ctrl+C"); err != nil {
				fmt.Fprintf(r.out, "Soft stop failed: %v\n", err)
			}
			return
		}
		fmt.Fprintln(r.out, "\nSecond Ctrl+C detected – aborting session.")
		if err := r.handle.Abort(); err != nil {
			fmt.Fprintf(r.out, "Abort failed: %v\n", err)
		}
	case syscall.SIGTERM, syscall.SIGHUP:
		fmt.Fprintf(r.out, "\nReceived %s – aborting session immediately.\n", sig)
		if err := r.handle.Abort(); err != nil {
			fmt.Fprintf(r.out, "Abort failed: %v\n", err)
		}
	}
}

func startSignalRelay(handle signalSession, signals <-chan os.Signal, out io.Writer) func() {
	if handle == nil || signals == nil {
		return func() {}
	}
	relay := newSignalRelay(handle, out)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				relay.handleSignal(sig)
			}
		}
	}()
	return func() {
		close(done)
	}
}
