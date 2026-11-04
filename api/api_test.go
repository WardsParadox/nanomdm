package api

import (
	"context"
	"errors"
	"testing"

	"github.com/micromdm/nanomdm/mdm"
	"github.com/micromdm/nanomdm/push"
)

// testEnqueuer enqueues commands and returns an error for every enrollment
// ID in errIDs.
type testEnqueuer struct {
	errIDs map[string]error
	err    error
}

func (e *testEnqueuer) EnqueueCommand(_ context.Context, ids []string, _ *mdm.Command) (map[string]error, error) {
	idErrs := make(map[string]error)
	for _, id := range ids {
		if err, ok := e.errIDs[id]; ok {
			idErrs[id] = err
		}
	}
	if len(idErrs) < 1 {
		idErrs = nil
	}
	return idErrs, e.err
}

// testPusher records the enrollment IDs it was asked to push to.
type testPusher struct {
	pushedIDs []string
	called    bool
}

func (p *testPusher) Push(_ context.Context, ids []string) (map[string]*push.Response, error) {
	p.called = true
	p.pushedIDs = append(p.pushedIDs, ids...)
	resp := make(map[string]*push.Response)
	for _, id := range ids {
		resp[id] = &push.Response{Id: "apns-id-" + id}
	}
	return resp, nil
}

func newTestCommand() *mdm.Command {
	cmd := &mdm.Command{CommandUUID: "test-uuid"}
	cmd.Command.RequestType = "DeviceInformation"
	return cmd
}

// sameIDs compares two sets of enrollment IDs ignoring order.
func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int)
	for _, id := range a {
		set[id]++
	}
	for _, id := range b {
		set[id]--
		if set[id] < 0 {
			return false
		}
	}
	return true
}

func TestEnqueueWithPushOnlyPushesEnqueuedIDs(t *testing.T) {
	for _, test := range []struct {
		name       string
		ids        []string
		errIDs     map[string]error
		wantPushed []string
		wantPush   bool
		wantCode   int
	}{
		{
			name:       "all enqueued",
			ids:        []string{"A", "B", "C"},
			wantPushed: []string{"A", "B", "C"},
			wantPush:   true,
			wantCode:   200,
		},
		{
			name:       "one enqueue error",
			ids:        []string{"A", "B", "C"},
			errIDs:     map[string]error{"B": errors.New("enqueue failed")},
			wantPushed: []string{"A", "C"},
			wantPush:   true,
			wantCode:   207,
		},
		{
			name:     "all enqueue errors",
			ids:      []string{"A", "B"},
			errIDs:   map[string]error{"A": errors.New("enqueue failed"), "B": errors.New("enqueue failed")},
			wantPush: false,
			wantCode: 500,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pusher := new(testPusher)
			pe, err := NewPushEnqueuer(&testEnqueuer{errIDs: test.errIDs}, pusher)
			if err != nil {
				t.Fatal(err)
			}

			r, code, err := pe.EnqueueWithPush(context.Background(), newTestCommand(), test.ids, false)
			if err != nil {
				t.Fatal(err)
			}

			if have, want := pusher.called, test.wantPush; have != want {
				t.Errorf("pusher called: have %v, want %v", have, want)
			}

			if !sameIDs(pusher.pushedIDs, test.wantPushed) {
				t.Errorf("pushed IDs: have %v, want %v", pusher.pushedIDs, test.wantPushed)
			}

			if have, want := code, test.wantCode; have != want {
				t.Errorf("code: have %d, want %d", have, want)
			}

			// an enrollment ID that failed to enqueue must not have a push result
			for id := range test.errIDs {
				if r.Status[id].PushID != "" {
					t.Errorf("enrollment ID %s had enqueue error but got push ID %s", id, r.Status[id].PushID)
				}
			}
		})
	}
}

// TestPushOnlyPushesAll makes sure a push-only call still pushes to every
// enrollment ID, because nothing was enqueued.
func TestPushOnlyPushesAll(t *testing.T) {
	pusher := new(testPusher)
	pe, err := NewPushEnqueuer(&testEnqueuer{}, pusher)
	if err != nil {
		t.Fatal(err)
	}

	ids := []string{"A", "B", "C"}
	_, code, err := pe.Push(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}

	if !sameIDs(pusher.pushedIDs, ids) {
		t.Errorf("pushed IDs: have %v, want %v", pusher.pushedIDs, ids)
	}

	if have, want := code, 200; have != want {
		t.Errorf("code: have %d, want %d", have, want)
	}
}
