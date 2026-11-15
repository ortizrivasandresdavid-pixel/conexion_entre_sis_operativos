package shell

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteHelp(t *testing.T) {
	output, err := Execute(context.Background(), "help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "whoami") {
		t.Fatalf("help output does not list diagnostics: %q", output)
	}
	if !strings.Contains(output, "ls -l") {
		t.Fatalf("help output does not list the fixed directory listing command: %q", output)
	}
}

func TestExecuteRejectsArbitraryInput(t *testing.T) {
	for _, command := range []string{"id", "uname -a; whoami", "df -h /tmp", "ls -l /etc", "pwd extra"} {
		if _, err := Execute(context.Background(), command); err == nil {
			t.Errorf("command %q was accepted", command)
		}
	}
}

func TestExecuteAllowsFixedDirectoryListing(t *testing.T) {
	_, err := Execute(context.Background(), "ls -l")
	if err != nil && strings.Contains(err.Error(), "comando no permitido") {
		t.Fatalf("the fixed directory listing command was rejected: %v", err)
	}
}
