package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestWatchTerminationSignalRequestsTrayExit(t *testing.T) {
	signals := make(chan os.Signal, 1)
	stop := make(chan struct{})
	received := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		waitForWatchTerminationSignal(signals, stop, func(sig os.Signal) {
			received <- sig
		})
		close(done)
	}()

	signals <- syscall.SIGINT
	select {
	case got := <-received:
		if got != syscall.SIGINT {
			t.Fatalf("signal = %v, want %v", got, syscall.SIGINT)
		}
	case <-time.After(time.Second):
		t.Fatal("termination signal did not request tray exit")
	}
	<-done
}

func TestWatchTerminationSignalHandlerStopsAfterTrayExit(t *testing.T) {
	signals := make(chan os.Signal, 1)
	stop := make(chan struct{})
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		waitForWatchTerminationSignal(signals, stop, func(os.Signal) {
			called <- struct{}{}
		})
		close(done)
	}()

	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("signal handler did not stop after tray exit")
	}
	select {
	case <-called:
		t.Fatal("normal tray exit requested another exit")
	default:
	}
}
