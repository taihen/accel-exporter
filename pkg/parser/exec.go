package parser

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// runAccelCmd runs accel-cmd with args and returns its stdout. A positive
// timeout bounds the whole call.
func runAccelCmd(accelCmdPath string, timeout time.Duration, args ...string) (string, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

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
