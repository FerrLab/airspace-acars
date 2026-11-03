package gsx

import (
	"testing"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func feed(t *testing.T, tr *tracker, msg string) ([]any, bool) {
	t.Helper()
	replies, changed, err := tr.handle([]byte(msg))
	require.NoError(t, err, msg)
	return replies, changed
}

// The handshake: hello is answered with a subscription, and an accepted
// subscription with a request for the state.
func TestHelloIsAnsweredWithASubscription(t *testing.T) {
	tr := newTracker()
	replies, changed := feed(t, tr, `{"v":1,"type":"hello","protocol":1,"engine":"gsx","gsxRunning":true,"authRequired":false,"capabilities":["state"]}`)
	require.Equal(t, []any{subscribeRequest}, replies)
	assert.True(t, changed)
	assert.True(t, tr.state().Running)

	replies, _ = feed(t, tr, `{"v":1,"type":"result","id":"subscribe","ok":true}`)
	assert.Equal(t, []any{snapshotRequest}, replies)
	assert.True(t, tr.subscribed)
}

func TestARefusedSubscriptionIsNoted(t *testing.T) {
	tr := newTracker()
	feed(t, tr, `{"type":"result","id":"subscribe","ok":false,"error":{"code":"auth"}}`)
	assert.True(t, tr.rejected)
	assert.False(t, tr.subscribed)
}

// Snapshots and patches each replace a whole top-level key. The variants the
// server is known or suspected to send are all read the same way.
func TestServicesAreReadFromSnapshotsAndPatches(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"snapshot", `{"type":"snapshot","gsxRunning":true,"services":[{"id":"Boarding","stateRaw":5,"canTrigger":false,"operator":"Swissport","progressText":"45/180"}]}`},
		{"snapshot nested under state", `{"type":"snapshot","state":{"services":[{"id":"Boarding","stateRaw":5,"progressText":"45/180"}]}}`},
		{"patch", `{"type":"patch","path":"/services","value":[{"id":"Boarding","stateRaw":5,"progressText":"45/180"}]}`},
		{"state as a string", `{"type":"patch","path":"/services","value":[{"id":"boarding","stateRaw":"5","progressText":"45/180"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTracker()
			_, changed := feed(t, tr, c.msg)
			assert.True(t, changed)
			assert.Equal(t, domain.GSXService{State: domain.GSXActive, Progress: "45/180"}, tr.state().Boarding)
		})
	}
}

func TestUnknownServicesAndMessagesAreIgnored(t *testing.T) {
	tr := newTracker()
	_, changed := feed(t, tr, `{"type":"patch","path":"/services","value":[{"id":"Catering","stateRaw":5}]}`)
	assert.False(t, changed)
	_, changed = feed(t, tr, `{"type":"event","name":"something new","extra":{"a":1}}`)
	assert.False(t, changed)
	_, changed = feed(t, tr, `{"type":"patch","path":"/services","value":"garbage"}`)
	assert.False(t, changed)
}

func TestGarbageIsAnError(t *testing.T) {
	_, _, err := newTracker().handle([]byte("not json"))
	assert.ErrorIs(t, err, errNotJSON)
}

// GSX does not always report "completed": a finished boarding can drop
// straight back to callable. Either counts, and calling it again clears it.
func TestServiceCompletionIsLatched(t *testing.T) {
	steps := []struct {
		state         domain.GSXServiceState
		wantCompleted bool
	}{
		{domain.GSXCallable, false},
		{domain.GSXRequested, false},
		{domain.GSXActive, false},
		{domain.GSXCallable, true}, // dropped out of active: done
		{domain.GSXNotAvailable, true},
		{domain.GSXRequested, false}, // called again
		{domain.GSXCompleted, true},
		{domain.GSXCallable, true},
	}
	var s domain.GSXService
	for i, step := range steps {
		s = advance(s, step.state, "")
		assert.Equal(t, step.wantCompleted, s.Completed, "step %d (state %d)", i, step.state)
	}
}

func TestADeepPatchAsksForASnapshot(t *testing.T) {
	replies, changed := feed(t, newTracker(), `{"type":"patch","path":"/services/0/stateRaw","value":6}`)
	assert.False(t, changed)
	assert.Equal(t, []any{snapshotRequest}, replies)
}
