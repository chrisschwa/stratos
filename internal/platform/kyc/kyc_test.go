package kyc

import (
	"context"
	"testing"
)

type fakeStore struct {
	pending []string
	set     map[string]Status
}

func (f *fakeStore) PendingValidationIDs(context.Context) ([]string, error) { return f.pending, nil }
func (f *fakeStore) SetValidationStatus(_ context.Context, id string, s Status) error {
	if f.set == nil {
		f.set = map[string]Status{}
	}
	f.set[id] = s
	return nil
}

type fakeProvider map[string]Status

func (p fakeProvider) CheckStatus(_ context.Context, id string) (Status, error) { return p[id], nil }

func TestScanAdvancesResolvedValidations(t *testing.T) {
	store := &fakeStore{pending: []string{"v1", "v2", "v3"}}
	prov := fakeProvider{"v1": StatusApproved, "v2": StatusPending, "v3": StatusRejected} // v2 stays pending
	n, err := NewScanJob(store, prov).Run(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("advanced=%d err=%v, want 2", n, err)
	}
	if store.set["v1"] != StatusApproved || store.set["v3"] != StatusRejected {
		t.Fatalf("statuses: %+v", store.set)
	}
	if _, touched := store.set["v2"]; touched {
		t.Fatal("v2 (pending) should not be updated")
	}
}

func TestStubProviderIsNoop(t *testing.T) {
	store := &fakeStore{pending: []string{"v1"}}
	n, err := NewScanJob(store, nil).Run(context.Background()) // nil → StubProvider
	if err != nil || n != 0 {
		t.Fatalf("stub should advance nothing: n=%d err=%v", n, err)
	}
}
