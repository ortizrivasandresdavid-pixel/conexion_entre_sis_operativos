package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

func isPortListening(port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		found := containsListenPort(file, port)
		_ = file.Close()
		if found {
			return true
		}
	}
	return false
}

func containsListenPort(reader io.Reader, port int) bool {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		separator := strings.LastIndexByte(fields[1], ':')
		if separator < 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[1][separator+1:], 16, 16)
		if err == nil && int(value) == port {
			return true
		}
	}
	return false
}

func formatAddress(ip string, port int) string {
	return net.JoinHostPort(ip, strconv.Itoa(port))
}
