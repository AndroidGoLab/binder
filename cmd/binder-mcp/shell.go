//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// shellExec runs a shell command and returns the combined stdout+stderr output.
func shellExec(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		return result, fmt.Errorf("command %q: %w (output: %s)", command, err, result)
	}
	return result, nil
}

// shellQuote quotes a string for safe use in shell commands,
// preventing injection attacks.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
