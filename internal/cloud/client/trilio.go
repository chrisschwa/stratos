package client

import (
	"context"
	"strings"
)

// trilio.go = the Trilio (TrilioVault backup) surface on the CloudClient facade.
// Trilio is the OpenStack "workloads" service (workload backups → snapshots → restores). It is
// ABSENT from the current regions, so these are CODE-ONLY / live-blocked: every call resolves the
// "workloads" endpoint from the catalog (errors cleanly when the service isn't present) + direct-RESTs
// with the provider's project-scoped token. Responses are returned as free-form maps (the SDK shape
// never leaks); single-object responses are unwrapped from their {key:{…}} envelope.

// trilioURL builds a Trilio REST URL under the "workloads" service endpoint.
func (c *Client) trilioURL(path string) (string, error) {
	base, err := c.EndpointURL("workloads")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base, "/") + path, nil
}

// trilioGet does a GET and unwraps resp[key] (the single-object envelope) when present.
func (c *Client) trilioGet(ctx context.Context, path, key string) (map[string]any, error) {
	url, err := c.trilioURL(path)
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := c.Do(ctx, "GET", url, nil, &resp, 200); err != nil {
		return nil, err
	}
	return trilioUnwrap(resp, key), nil
}

// trilioList does a GET and returns resp[key] as a list of maps (the {key:[…]} envelope).
func (c *Client) trilioList(ctx context.Context, path, key string) ([]map[string]any, error) {
	url, err := c.trilioURL(path)
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := c.Do(ctx, "GET", url, nil, &resp, 200); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	if arr, ok := resp[key].([]any); ok {
		for _, x := range arr {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

// trilioSend does a POST/PUT with body + unwraps resp[key]. okCodes default 200/202.
func (c *Client) trilioSend(ctx context.Context, method, path string, body map[string]any, key string) (map[string]any, error) {
	url, err := c.trilioURL(path)
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := c.Do(ctx, method, url, body, &resp, 200, 202); err != nil {
		return nil, err
	}
	return trilioUnwrap(resp, key), nil
}

// trilioVoid does a method with no response body (delete / cancel / unlock / dismount).
func (c *Client) trilioVoid(ctx context.Context, method, path string, body map[string]any) error {
	url, err := c.trilioURL(path)
	if err != nil {
		return err
	}
	return c.Do(ctx, method, url, body, nil, 200, 202, 204)
}

func trilioUnwrap(resp map[string]any, key string) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	if m, ok := resp[key].(map[string]any); ok {
		return m
	}
	return resp
}

// --- Workloads ---

func (c *Client) ListWorkloads(ctx context.Context) ([]map[string]any, error) {
	return c.trilioList(ctx, "/workloads/detail", "workloads")
}
func (c *Client) GetWorkload(ctx context.Context, id string) (map[string]any, error) {
	return c.trilioGet(ctx, "/workloads/"+id, "workload")
}
func (c *Client) CreateWorkload(ctx context.Context, payload map[string]any) (map[string]any, error) {
	return c.trilioSend(ctx, "POST", "/workloads", map[string]any{"workload": payload}, "workload")
}
func (c *Client) UpdateWorkload(ctx context.Context, id string, payload map[string]any) (map[string]any, error) {
	return c.trilioSend(ctx, "PUT", "/workloads/"+id, map[string]any{"workload": payload}, "workload")
}
func (c *Client) DeleteWorkload(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "DELETE", "/workloads/"+id, nil)
}

// --- Snapshots ---

func (c *Client) ListSnapshots(ctx context.Context) ([]map[string]any, error) {
	return c.trilioList(ctx, "/snapshots/detail", "snapshots")
}
func (c *Client) GetSnapshot(ctx context.Context, id string) (map[string]any, error) {
	return c.trilioGet(ctx, "/snapshots/"+id, "snapshot")
}

// CreateSnapshot takes a snapshot of a workload (POST /workloads/{id}/snapshots?full=).
func (c *Client) CreateSnapshot(ctx context.Context, workloadID string, full bool, payload map[string]any) (map[string]any, error) {
	q := "?full=false"
	if full {
		q = "?full=true"
	}
	return c.trilioSend(ctx, "POST", "/workloads/"+workloadID+"/snapshots"+q, map[string]any{"snapshot": payload}, "snapshot")
}
func (c *Client) DeleteSnapshot(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "DELETE", "/snapshots/"+id, nil)
}
func (c *Client) CancelSnapshot(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "GET", "/snapshots/"+id+"/cancel", nil)
}
func (c *Client) MountSnapshot(ctx context.Context, snapshotID, mountVMID string) error {
	return c.trilioVoid(ctx, "POST", "/snapshots/"+snapshotID+"/mount",
		map[string]any{"mount": map[string]any{"mount_vm_id": mountVMID}})
}
func (c *Client) DismountSnapshot(ctx context.Context, snapshotID string) error {
	return c.trilioVoid(ctx, "POST", "/snapshots/"+snapshotID+"/dismount", map[string]any{"dismount": map[string]any{}})
}
func (c *Client) ListMountedSnapshots(ctx context.Context, workloadID string) ([]map[string]any, error) {
	return c.trilioList(ctx, "/workloads/"+workloadID+"/snapshots/mounted/list", "mounted_snapshots")
}

// --- Restores ---

func (c *Client) ListRestores(ctx context.Context) ([]map[string]any, error) {
	return c.trilioList(ctx, "/restores/detail", "restores")
}
func (c *Client) GetRestore(ctx context.Context, id string) (map[string]any, error) {
	return c.trilioGet(ctx, "/restores/"+id, "restore")
}
func (c *Client) CreateRestore(ctx context.Context, snapshotID string, payload map[string]any) (map[string]any, error) {
	return c.trilioSend(ctx, "POST", "/snapshots/"+snapshotID, map[string]any{"restore": payload}, "restore")
}
func (c *Client) DeleteRestore(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "DELETE", "/restores/"+id, nil)
}
func (c *Client) CancelRestore(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "GET", "/restores/"+id+"/cancel", nil)
}

// --- Backup targets + file search ---

func (c *Client) ListBackupTargetTypes(ctx context.Context) ([]map[string]any, error) {
	return c.trilioList(ctx, "/backup_target_types/detail", "backup_target_types")
}
func (c *Client) ListBackupTargets(ctx context.Context) ([]map[string]any, error) {
	return c.trilioList(ctx, "/backup_targets/detail", "backup_targets")
}
func (c *Client) GetBackupTarget(ctx context.Context, id string) (map[string]any, error) {
	return c.trilioGet(ctx, "/backup_targets/"+id, "backup_target")
}
func (c *Client) CreateBackupTarget(ctx context.Context, payload map[string]any) (map[string]any, error) {
	return c.trilioSend(ctx, "POST", "/backup_targets", map[string]any{"backup_target": payload}, "backup_target")
}
func (c *Client) DeleteBackupTarget(ctx context.Context, id string) error {
	return c.trilioVoid(ctx, "DELETE", "/backup_targets/"+id, nil)
}
func (c *Client) StartFileSearch(ctx context.Context, payload map[string]any) (map[string]any, error) {
	return c.trilioSend(ctx, "POST", "/search", map[string]any{"file_search": payload}, "file_search")
}
func (c *Client) GetFileSearchResults(ctx context.Context, searchID string) (map[string]any, error) {
	return c.trilioGet(ctx, "/search/"+searchID, "file_search")
}
