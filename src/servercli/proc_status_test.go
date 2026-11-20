package main

import (
	"strings"
	"testing"
)

func TestContainsListenPort(t *testing.T) {
	table := "sl local_address rem_address st\n" +
		"0: 00000000:08FE 00000000:0000 0A\n" +
		"1: 00000000:08FF 00000000:0000 01\n"
	if !containsListenPort(strings.NewReader(table), 2302) {
		t.Fatal("no se detectó el puerto TCP en LISTEN")
	}
	if containsListenPort(strings.NewReader(table), 2303) {
		t.Fatal("se detectó un puerto que no está en LISTEN")
	}
}

func TestFormatAddressSupportsIPv6(t *testing.T) {
	if got, want := formatAddress("::1", 2306), "[::1]:2306"; got != want {
		t.Fatalf("formatAddress() = %q; se esperaba %q", got, want)
	}
}
