package main

import "testing"

func TestListenRejectsUnexpectedSocketPath(t *testing.T) {
	listener, err := listen("/tmp/attacker-controlled.sock")
	if listener != nil {
		_ = listener.Close()
		t.Fatal("listen() returned a listener for an unexpected path")
	}
	if err == nil {
		t.Fatal("listen() accepted an unexpected socket path")
	}
}
