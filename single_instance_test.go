package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// withSingleInstanceLock stands in for the platform's answer to whether the
// single-instance lock can be held, and restores it when the test ends.
func withSingleInstanceLock(t *testing.T, available bool) {
	t.Helper()
	orig := singleInstanceLockAvailable
	singleInstanceLockAvailable = func() bool { return available }
	t.Cleanup(func() { singleInstanceLockAvailable = orig })
}

// The ACARS used to find a running copy by listening on 127.0.0.1:49876 and
// took any failure to listen for one, so a port Windows had reserved, or any
// program sitting on it, kept the ACARS from starting at all.
func TestAProgramOnTheOldPortDoesNotStopTheACARSStarting(t *testing.T) {
	withSingleInstanceLock(t, true)
	l, err := net.Listen("tcp", "127.0.0.1:49876")
	if err != nil {
		t.Skipf("cannot hold the old port for the test: %v", err)
	}
	defer l.Close()

	opts := singleInstance(func() {})
	require.NotNil(t, opts, "with 127.0.0.1:49876 taken the ACARS starts without its single-instance guard")
	assert.Equal(t, singleInstanceID, opts.UniqueID)
}

// A second launch hands over to the copy already open, which brings its
// window forward instead of the pilot getting a second ACARS.
func TestASecondLaunchBringsTheOpenWindowForward(t *testing.T) {
	withSingleInstanceLock(t, true)
	shown := 0
	opts := singleInstance(func() { shown++ })
	require.NotNil(t, opts)
	require.NotNil(t, opts.OnSecondInstanceLaunch)

	opts.OnSecondInstanceLaunch(application.SecondInstanceData{})
	assert.Equal(t, 1, shown, "a second launch did not bring the open window forward")
}

// Wails stops an application whose lock it cannot take, so where the lock is
// unavailable the ACARS starts without the guard rather than not at all.
func TestTheACARSStartsUnguardedWhereTheLockIsUnavailable(t *testing.T) {
	withSingleInstanceLock(t, false)
	assert.Nil(t, singleInstance(func() {}), "the ACARS would refuse to start on a machine that cannot hold the lock")
}
