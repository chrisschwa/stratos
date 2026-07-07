// Package kyc is the identity-verification (KYC) integration (the periodic KYC scan + the
// idenfy/verification integrations). No vendor is purchased yet, so the default Provider is a
// StubProvider (no-op): the structure, scan job, and tests exist and pass; a real vendor plugs in
// later by implementing Provider + wiring the cron + repo. This is the "implement + test, integrate
// later" slice the product owner asked for.
package kyc

import "context"

// Status is the vendor's verification outcome for a submitted identity validation.
type Status string

const (
	StatusPending  Status = "PENDING"
	StatusApproved Status = "APPROVED"
	StatusRejected Status = "REJECTED"
)

// Provider checks/advances an identity validation with an external KYC vendor.
type Provider interface {
	// CheckStatus returns the vendor's current status for a submitted validation. An empty status
	// means "no change / not configured" (the StubProvider) — the scan leaves the record untouched.
	CheckStatus(ctx context.Context, validationID string) (Status, error)
}

// StubProvider is the default until a vendor is integrated: it advances nothing.
type StubProvider struct{}

func (StubProvider) CheckStatus(context.Context, string) (Status, error) { return "", nil }

// Store is the persistence the scan needs (a billing.Repo adapter binds this when a vendor lands).
type Store interface {
	PendingValidationIDs(ctx context.Context) ([]string, error)
	SetValidationStatus(ctx context.Context, id string, status Status) error
}

// ScanJob polls pending identity validations and advances their status from the vendor. With
// the StubProvider it is a no-op (the vendor returns "" → nothing updated).
type ScanJob struct {
	store    Store
	provider Provider
}

func NewScanJob(store Store, provider Provider) *ScanJob {
	if provider == nil {
		provider = StubProvider{}
	}
	return &ScanJob{store: store, provider: provider}
}

// Run scans pending validations, querying the vendor for each; returns the number advanced.
func (j *ScanJob) Run(ctx context.Context) (int, error) {
	ids, err := j.store.PendingValidationIDs(ctx)
	if err != nil {
		return 0, err
	}
	advanced := 0
	for _, id := range ids {
		status, err := j.provider.CheckStatus(ctx, id)
		if err != nil {
			continue // best-effort per record (a vendor error must not abort the scan)
		}
		if status == "" || status == StatusPending {
			continue
		}
		if err := j.store.SetValidationStatus(ctx, id, status); err != nil {
			return advanced, err
		}
		advanced++
	}
	return advanced, nil
}
