// Package metrics is the OpenStack Gnocchi usage path, implemented as a
// DIRECT-REST client — gophercloud has no Gnocchi (metric) service, so calls go through
// the CloudClient's authenticated transport. Input: the billable network traffic per
// server, in MB, for the current month.
package metrics

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/cockroachdb/apd/v3"
	"github.com/shopspring/decimal"

	"github.com/menlocloud/stratos/internal/cloud/client"
)

// DefaultGranularity is used when the
// region's openstackConfig has none. resample is fixed at 3600 (1h).
const (
	DefaultGranularity = 300
	resample           = "3600"
	bytesPerMB         = 1048576
)

// decimal64 = 16 significant digits, HALF_EVEN. shopspring
// rounds by decimal places, not significant digits, so the MB division uses apd for
// significant-digit division before feeding the rating engine.
var decimal64 = apd.Context{Precision: 16, Rounding: apd.RoundHalfEven, MaxExponent: apd.MaxExponent, MinExponent: apd.MinExponent}

// Gnocchi is the direct-REST metric client bound to one cloud scope.
type Gnocchi struct {
	cc   *client.Client
	base string // the "metric" service public endpoint
}

// New resolves the Gnocchi endpoint from the catalog and returns the client.
func New(cc *client.Client) (*Gnocchi, error) {
	base, err := cc.EndpointURL("metric")
	if err != nil {
		return nil, fmt.Errorf("gnocchi: locate metric endpoint: %w", err)
	}
	return &Gnocchi{cc: cc, base: trimSlash(base)}, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// Ping is a read-only connectivity check (GET /v1/metric?limit=1) — proves the resolved
// endpoint + token work, for the live smoke. Creates nothing.
func (g *Gnocchi) Ping(ctx context.Context) error {
	var out []any
	return g.cc.Do(ctx, "GET", g.base+"/v1/metric?limit=1", nil, &out, 200)
}

// Resource is a Gnocchi resource (subset): its id, name (the tap device for a network
// interface), and the metric-name → metric-id map.
type Resource struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Metrics map[string]string `json:"metrics"`
}

// SearchInstanceInterfaces lists a server's network-interface resources:
// POST /v1/search/resource/instance_network_interface
// with body {"=":{"instance_id": <novaUuid>}}. Each result's Metrics map holds the
// network.incoming.bytes / network.outgoing.bytes metric ids.
func (g *Gnocchi) SearchInstanceInterfaces(ctx context.Context, instanceID string) ([]Resource, error) {
	body := map[string]any{"=": map[string]any{"instance_id": instanceID}}
	var out []Resource
	if err := g.cc.Do(ctx, "POST", g.base+"/v1/search/resource/instance_network_interface", body, &out, 200); err != nil {
		return nil, err
	}
	return out, nil
}

// MeasuresMBForCurrentMonth GETs the
// per-metric measures over the month and returns the billable traffic in MB =
// (max(value) − min(value)) / 1048576 (a cumulative-counter delta, decimal64 division).
// granularity ≤ 0 falls back to DefaultGranularity.
func (g *Gnocchi) MeasuresMBForCurrentMonth(ctx context.Context, metricID string, granularity int, start time.Time) (decimal.Decimal, error) {
	if granularity <= 0 {
		granularity = DefaultGranularity
	}
	q := url.Values{}
	q.Set("granularity", strconv.Itoa(granularity))
	q.Set("resample", resample)
	q.Set("start", start.UTC().Format(time.RFC3339Nano))
	// Gnocchi measures = array of [timestamp, granularity, value] rows.
	var rows [][]any
	u := g.base + "/v1/metric/" + url.PathEscape(metricID) + "/measures?" + q.Encode()
	if err := g.cc.Do(ctx, "GET", u, nil, &rows, 200); err != nil {
		return decimal.Zero, err
	}
	return measuresDeltaMB(rows)
}

// measuresDeltaMB computes (max − min)/1048576 over the measure values (index 2 of each
// row), DECIMAL64. Extracted for unit testing without a live cloud.
func measuresDeltaMB(rows [][]any) (decimal.Decimal, error) {
	var maxV, minV *decimal.Decimal
	for _, row := range rows {
		if len(row) < 3 || row[2] == nil {
			continue
		}
		v, err := toDecimal(row[2])
		if err != nil {
			return decimal.Zero, err
		}
		if maxV == nil || v.GreaterThan(*maxV) {
			cp := v
			maxV = &cp
		}
		if minV == nil || v.LessThan(*minV) {
			cp := v
			minV = &cp
		}
	}
	if maxV == nil {
		return decimal.Zero, nil // no measures → 0 (max/min defaults to 0 → 0/1MiB = 0)
	}
	deltaBytes := maxV.Sub(*minV)
	return divDecimal64(deltaBytes, decimal.NewFromInt(bytesPerMB))
}

func toDecimal(v any) (decimal.Decimal, error) {
	switch n := v.(type) {
	case float64:
		// JSON numbers decode to float64 via encoding/json; format then parse to avoid
		// binary-float drift (gnocchi byte counters are integers, so this is exact).
		return decimal.NewFromString(strconv.FormatFloat(n, 'f', -1, 64))
	case string:
		return decimal.NewFromString(n)
	default:
		return decimal.Zero, fmt.Errorf("gnocchi: unexpected measure value type %T", v)
	}
}

// divDecimal64 = a / b under MathContext.DECIMAL64 (16 sig digits, HALF_EVEN).
func divDecimal64(a, b decimal.Decimal) (decimal.Decimal, error) {
	if b.IsZero() {
		return decimal.Zero, fmt.Errorf("gnocchi: divide by zero")
	}
	ad, _, err := apd.NewFromString(a.String())
	if err != nil {
		return decimal.Zero, err
	}
	bd, _, err := apd.NewFromString(b.String())
	if err != nil {
		return decimal.Zero, err
	}
	res := new(apd.Decimal)
	if _, err := decimal64.Quo(res, ad, bd); err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(res.Text('f'))
}
