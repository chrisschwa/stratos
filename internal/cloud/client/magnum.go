package client

import (
	"context"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/containerinfra/v1/certificates"
	"github.com/gophercloud/gophercloud/v2/openstack/containerinfra/v1/clusters"
	"github.com/gophercloud/gophercloud/v2/openstack/containerinfra/v1/clustertemplates"
	"github.com/gophercloud/gophercloud/v2/openstack/containerinfra/v1/nodegroups"
)

// magnum.go = the Magnum (container-infra) Kubernetes-cluster read/write surface on the CloudClient
// facade. A cluster is the
// KUBERNETES_CLUSTER CloudResource; its externalId == the Magnum cluster UUID. data =
// DataCluster{cluster, nodeGroups}. Each call returns the object as a free-form map[string]any
// (the CloudResource.data shape) so the gophercloud type never leaks.
//
// NOTE: there is no Magnum backend on the dev region — this is code-only, live-blocked (the cluster
// catalog does not enable container-infra).

func (c *Client) magnum() (*gophercloud.ServiceClient, error) {
	return openstack.NewContainerInfraV1(c.provider, c.endpointOpts())
}

// magnumLabels parses a flat "k1=v1, k2=v2" string → map. Blank → empty map;
// non "k=v" pairs are dropped (split on ", " then "=").
func magnumLabels(labels string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(labels) == "" {
		return out
	}
	for _, pair := range strings.Split(labels, ", ") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func intPtr(i int) *int    { return &i }
func boolPtr(b bool) *bool { return &b }

// CreateClusterOpts mirrors CreateClusterRequest. The
// flavor/template/network ids are the required quartet (validated upstream); HA/autoscaling/floating-ip
// toggles drive master_count + the boot/scaling labels.
type CreateClusterOpts struct {
	Name                        string
	KeyName                     string // → keypair
	ClusterTemplateID           string
	WorkerFlavorID              string // → flavor_id
	MasterFlavorID              string
	NodeCount                   int
	FixedNetwork                string // resolved NETWORK externalId
	HA                          bool
	FloatingIP                  bool
	Autoscaling                 bool
	MinNodeCount                string // label values are strings
	MaxNodeCount                string
	ContainerVolumesStorageType string // → boot_volume_type label
	ContainerVolumesStorageSize string // → boot_volume_size label
	Labels                      string // raw "k=v, k=v" user labels
}

// CreateCluster creates a Magnum cluster then re-fetches it (create → show) so the cached
// data carries the full, server-populated cluster. clusters.Create returns only the UUID.
func (c *Client) CreateCluster(ctx context.Context, o CreateClusterOpts) (map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}

	labels := magnumLabels(o.Labels)
	opts := clusters.CreateOpts{
		Name:              o.Name,
		ClusterTemplateID: o.ClusterTemplateID,
		Keypair:           o.KeyName,
		NodeCount:         intPtr(o.NodeCount),
		FlavorID:          o.WorkerFlavorID,
		MasterFlavorID:    o.MasterFlavorID,
		MergeLabels:       boolPtr(true),
	}
	if o.HA {
		opts.MasterCount = intPtr(3)
		opts.MasterLBEnabled = boolPtr(true)
		labels["master_lb_floating_ip_enabled"] = "true"
		labels["etcd_lb_disabled"] = "false"
	} else {
		opts.MasterCount = intPtr(1)
	}
	if o.Autoscaling {
		labels["auto_scaling_enabled"] = "true"
		labels["min_node_count"] = o.MinNodeCount
		labels["max_node_count"] = o.MaxNodeCount
	} else {
		labels["auto_scaling_enabled"] = "false"
	}
	labels["boot_volume_size"] = o.ContainerVolumesStorageSize
	labels["boot_volume_type"] = o.ContainerVolumesStorageType
	if o.FloatingIP {
		opts.FloatingIPEnabled = boolPtr(true)
	}
	if o.FixedNetwork != "" {
		opts.FixedNetwork = o.FixedNetwork
	}
	opts.Labels = labels

	uuid, err := clusters.Create(ctx, mc, opts).Extract()
	if err != nil {
		return nil, err
	}
	// Re-read the full cluster; fall back to a minimal {uuid} map if the read fails.
	if full, gerr := clusters.Get(ctx, mc, uuid).Extract(); gerr == nil && full != nil {
		return toMap(full), nil
	}
	return map[string]any{"uuid": uuid}, nil
}

// GetCluster fetches a Magnum cluster by UUID.
func (c *Client) GetCluster(ctx context.Context, id string) (map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	cl, err := clusters.Get(ctx, mc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(cl), nil
}

// ListClusters lists the project's Magnum clusters.
func (c *Client) ListClusters(ctx context.Context) ([]map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	pages, err := clusters.List(mc, clusters.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	cls, err := clusters.ExtractClusters(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cls))
	for i := range cls {
		out = append(out, toMap(cls[i]))
	}
	return out, nil
}

// DeleteCluster deletes a Magnum cluster by UUID.
func (c *Client) DeleteCluster(ctx context.Context, id string) error {
	mc, err := c.magnum()
	if err != nil {
		return err
	}
	return clusters.Delete(ctx, mc, id).ExtractErr()
}

// ListClusterNodeGroups lists a cluster's node groups (used to
// enrich the cached DataCluster.nodeGroups in toCloudResource/sync).
func (c *Client) ListClusterNodeGroups(ctx context.Context, clusterID string) ([]map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	pages, err := nodegroups.List(mc, clusterID, nodegroups.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ngs, err := nodegroups.ExtractNodeGroups(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ngs))
	for i := range ngs {
		out = append(out, toMap(ngs[i]))
	}
	return out, nil
}

// ListClusterTemplates lists the project's cluster templates (the LIST_TEMPLATES action).
func (c *Client) ListClusterTemplates(ctx context.Context) ([]map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	pages, err := clustertemplates.List(mc, clustertemplates.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := clustertemplates.ExtractClusterTemplates(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ts))
	for i := range ts {
		out = append(out, toMap(ts[i]))
	}
	return out, nil
}

// GetClusterTemplate fetches a single cluster template by id (the GET_TEMPLATE action; the provider
// reads data.cluster.cluster_template_id then this).
func (c *Client) GetClusterTemplate(ctx context.Context, templateID string) (map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	t, err := clustertemplates.Get(ctx, mc, templateID).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(t), nil
}

// ResizeCluster scales a cluster's node count (the RESIZE_CLUSTER
// action). Returns the cluster UUID.
func (c *Client) ResizeCluster(ctx context.Context, id string, nodeCount int) (string, error) {
	mc, err := c.magnum()
	if err != nil {
		return "", err
	}
	n := nodeCount
	return clusters.Resize(ctx, mc, id, clusters.ResizeOpts{NodeCount: &n}).Extract()
}

// UpgradeCluster upgrades a cluster to a new template (the
// UPGRADE_CLUSTER action; maxBatchSize=1). Returns the cluster UUID.
func (c *Client) UpgradeCluster(ctx context.Context, id, clusterTemplateID string) (string, error) {
	mc, err := c.magnum()
	if err != nil {
		return "", err
	}
	return clusters.Upgrade(ctx, mc, id, clusters.UpgradeOpts{
		ClusterTemplate: clusterTemplateID,
		MaxBatchSize:    intPtr(1),
	}).Extract()
}

// GetClusterCertificate fetches the cluster's CA certificate. The PEM is base64-encoded into the
// kubeconfig by the provider/action layer.
func (c *Client) GetClusterCertificate(ctx context.Context, clusterID string) (map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	cert, err := certificates.Get(ctx, mc, clusterID).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(cert), nil
}

// SignClusterCertificate signs a CSR for the cluster (magnum signCertificate{clusterUuid, csr}).
// Returns the signed certificate (PEM).
func (c *Client) SignClusterCertificate(ctx context.Context, clusterID, csr string) (map[string]any, error) {
	mc, err := c.magnum()
	if err != nil {
		return nil, err
	}
	cert, err := certificates.Create(ctx, mc, certificates.CreateOpts{
		ClusterUUID: clusterID,
		CSR:         csr,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(cert), nil
}
