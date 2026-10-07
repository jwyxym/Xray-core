package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestLibraryLifecycle(t *testing.T) {
	var lib library
	t.Cleanup(func() { lib.stop() })
	if lib.stop() != 0 {
		t.Fatal(lib.errorText())
	}
	if lib.startArgs(`["--set","outbounds[0].protocol=freedom"]`) != 0 {
		t.Fatal(lib.errorText())
	}
	if lib.startJSON(`{}`) != -1 || !strings.Contains(lib.errorText(), "already running") {
		t.Fatal("a second start must fail without replacing the running instance")
	}
	if lib.stop() != 0 || lib.errorText() != "" {
		t.Fatal(lib.errorText())
	}
	if lib.startJSON(`{"outbounds":[{"protocol":"freedom"}]}`) != 0 {
		t.Fatal(lib.errorText())
	}
}

func TestLibraryInvalidInput(t *testing.T) {
	var lib library
	for _, config := range []string{"", "null", "[]", "{", `{"inbounds":[{"protocol":"invalid"}]}`} {
		if lib.startJSON(config) != -1 || lib.errorText() == "" || lib.instance != nil {
			t.Fatalf("invalid config was accepted: %s", config)
		}
	}
	for _, args := range []string{"", "null", "[]", `{}`, `["--unknown"]`, `["-c","stdin:"]`, `["--set","inbounds[3].port=1080"]`} {
		if lib.startArgs(args) != -1 || lib.errorText() == "" || lib.instance != nil {
			t.Fatalf("invalid args were accepted: %s", args)
		}
	}
}

func TestLibraryStartFailureRecovery(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	var lib library
	t.Cleanup(func() { lib.stop() })
	config := fmt.Sprintf(`{"inbounds":[{"protocol":"socks","listen":"127.0.0.1","port":%d}],"outbounds":[{"protocol":"freedom"}]}`, port)
	if lib.startJSON(config) != -1 || lib.instance != nil {
		t.Fatal("binding an occupied port must fail")
	}
	if lib.startJSON(`{"outbounds":[{"protocol":"freedom"}]}`) != 0 {
		t.Fatal(lib.errorText())
	}
}

func TestLibraryOperationPanic(t *testing.T) {
	var lib library
	if lib.operation(func() error { panic("test panic") }) != -1 || !strings.Contains(lib.errorText(), "test panic") {
		t.Fatal("operation panic must become an error")
	}
	if lib.stop() != 0 {
		t.Fatal("operation mutex was not released")
	}
}
