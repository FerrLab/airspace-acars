//go:build linux

package main

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On Linux the lock is a name on the D-Bus session bus. The probe must say
// no without one, or Wails stops the ACARS at startup, and yes with one, or
// every Linux pilot silently loses the guard.

func TestNoSessionBusMeansNoLock(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+filepath.Join(t.TempDir(), "no-bus"))
	assert.False(t, platformSingleInstanceLockAvailable(), "without a session bus Wails would stop the ACARS at startup")
}

func TestASessionBusMakesTheLockAvailable(t *testing.T) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed")
	}
	addr := "unix:path=" + filepath.Join(t.TempDir(), "bus")
	cmd := exec.Command(daemon, "--session", "--nofork", "--nopidfile", "--address="+addr, "--print-address=1")
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		// The daemon belongs to this test; how it dies does not matter.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// The daemon prints its address once it accepts connections.
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err, "dbus-daemon did not start")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(line))

	assert.True(t, platformSingleInstanceLockAvailable(), "a Linux machine with a session bus runs without the single-instance guard")
}
