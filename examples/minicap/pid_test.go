package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStartupPID(t *testing.T) {
	if os.Getenv("MINICAP_PID_HELPER") == "1" {
		args := strings.Split(os.Getenv("MINICAP_PID_ARGS"), " ")
		_ = run(context.Background(), args, os.Stdout)
		return
	}
	for _, tc := range []struct {
		name string
		args string
		want bool
	}{
		{"stream", "-P 1x1@1x1/0", true},
		{"screenshot", "-P 1x1@1x1/0 -s", true},
		{"test", "-P 1x1@1x1/0 -t", true},
		{"raw", "-P 1x1@1x1/0 -raw", true},
		{"help", "-h", false},
		{"invalid", "-P invalid", false},
		{"info", "-i", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartupPID$")
			cmd.Env = append(os.Environ(), "MINICAP_PID_HELPER=1", "MINICAP_PID_ARGS="+tc.args)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("run helper: %v; stderr = %q", err, stderr.String())
			}
			pidLine := fmt.Sprintf("PID: %d\n", cmd.Process.Pid)
			if got := strings.Contains(stderr.String(), pidLine); got != tc.want {
				t.Errorf("PID line present = %v, want %v; stderr = %q", got, tc.want, stderr.String())
			}
			if bytes.Contains(stdout.Bytes(), []byte(pidLine)) {
				t.Errorf("PID line leaked into stdout")
			}
		})
	}
}
