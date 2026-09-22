package parser

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// runAccelCmd runs accel-cmd with args and returns its stdout. The call is
// bounded by ctx, so callers can share one deadline across several commands.
func runAccelCmd(ctx context.Context, accelCmdPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, accelCmdPath, args...)
	// WaitDelay bounds how long Run blocks after the context is cancelled and the
	// process killed. Without it, a child that forks (e.g. a shell wrapper that
	// spawns a long-running grandchild) can inherit the stdout pipe and keep it
	// open, leaving Run stuck reading until that grandchild exits.
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
