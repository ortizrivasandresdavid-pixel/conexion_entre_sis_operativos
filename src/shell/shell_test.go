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
}

func TestExecuteRejectsArbitraryInput(t *testing.T) {
	for _, command := range []string{"id", "uname -a; whoami", "df -h /tmp", "pwd extra"} {
		if _, err := Execute(context.Background(), command); err == nil {
			t.Errorf("command %q was accepted", command)
		}
	}
}
