package main

import (
	"bufio"
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
		if os.Getenv("MINICAP_PID_HOLD") == "1" {
			time.Sleep(time.Minute)
		}
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
			if tc.want {
				cmd.Env = append(cmd.Env, "MINICAP_PID_HOLD=1")
			}
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			stderr, err := cmd.StderrPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			pidLine := fmt.Sprintf("PID: %d\n", cmd.Process.Pid)
			if tc.want {
				line, err := bufio.NewReader(stderr).ReadString('\n')
				if err != nil || line != pidLine {
					t.Errorf("first stderr line = %q, error = %v; want %q", line, err, pidLine)
				}
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			} else {
				var output bytes.Buffer
				_, err := output.ReadFrom(stderr)
				if err != nil {
					t.Errorf("read stderr: %v", err)
				}
				if err := cmd.Wait(); err != nil {
					t.Errorf("run helper: %v; stderr = %q", err, output.String())
				}
				if strings.Contains(output.String(), pidLine) {
					t.Errorf("unexpected PID line in stderr: %q", output.String())
				}
			}
			if bytes.Contains(stdout.Bytes(), []byte(pidLine)) {
				t.Errorf("PID line leaked into stdout")
			}
		})
	}
}
