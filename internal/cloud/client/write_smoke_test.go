package client

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestWriteSmoke is a LIVE create→verify→cleanup smoke for the networking write surface.
// It builds an INTERNAL network + subnet, a router with an external gateway to the pre-existing
// "guest" network, and a router interface — then tears everything down (LIFO via t.Cleanup,
// so partial failures still clean up). It NEVER creates an external network.
//
// DOUBLE-GATED: runs only when BOTH OS_AUTH_URL and STRATOS_WRITE_SMOKE=1 are set, so neither the
// normal `go test ./...` nor the read-only TestCloudSmoke ever creates a resource. Run:
//
//	STRATOS_WRITE_SMOKE=1 OS_AUTH_URL=https://cloud-console.menlo.ai:5000/v3 OS_REGION_NAME=RegionOne \
//	OS_USERNAME=dev OS_PASSWORD=*** OS_USER_DOMAIN_NAME=Default \
//	OS_PROJECT_NAME=dev OS_PROJECT_DOMAIN_NAME=Default \
//	go test ./internal/cloud/client -run TestWriteSmoke -v
func TestWriteSmoke(t *testing.T) {
	if os.Getenv("OS_AUTH_URL") == "" || os.Getenv("STRATOS_WRITE_SMOKE") != "1" {
		t.Skip("OS_AUTH_URL + STRATOS_WRITE_SMOKE=1 required — skipping live write smoke")
	}
	cfg := Config{
		AuthURL: os.Getenv("OS_AUTH_URL"), Region: os.Getenv("OS_REGION_NAME"),
		Username: os.Getenv("OS_USERNAME"), Password: os.Getenv("OS_PASSWORD"),
		UserDomainName: os.Getenv("OS_USER_DOMAIN_NAME"),
		ProjectName:    os.Getenv("OS_PROJECT_NAME"), ProjectDomainName: os.Getenv("OS_PROJECT_DOMAIN_NAME"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	// 1. internal network
	net, err := c.CreateNetwork(ctx, CreateNetworkOpts{Name: "stratos-go-write-smoke"})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	netID, _ := net["id"].(string)
	if netID == "" {
		t.Fatalf("create network: no id in %v", net)
	}
	t.Cleanup(func() {
		if err := c.DeleteNetwork(context.Background(), netID); err != nil {
			t.Errorf("cleanup network %s: %v", netID, err)
		} else {
			t.Logf("deleted network %s", netID)
		}
	})
	t.Logf("created internal network %s", netID)

	// 2. subnet (auto gateway, dhcp on)
	dhcp := true
	sn, err := c.CreateSubnet(ctx, CreateSubnetOpts{
		NetworkID: netID, Name: "stratos-go-write-smoke-subnet",
		CIDR: "192.168.234.0/24", IPVersion: 4, EnableDHCP: &dhcp, Gateway: true,
	})
	if err != nil {
		t.Fatalf("create subnet: %v", err)
	}
	snID, _ := sn["id"].(string)
	if snID == "" {
		t.Fatalf("create subnet: no id in %v", sn)
	}
	t.Cleanup(func() {
		if err := c.DeleteSubnet(context.Background(), snID); err != nil {
			t.Errorf("cleanup subnet %s: %v", snID, err)
		} else {
			t.Logf("deleted subnet %s", snID)
		}
	})
	t.Logf("created subnet %s", snID)

	// 3. find the pre-existing external "guest" network (created by the operator, not us)
	exts, err := c.ListExternalNetworks(ctx)
	if err != nil {
		t.Fatalf("list external networks: %v", err)
	}
	guestID := ""
	for _, e := range exts {
		if e.Name == "guest" {
			guestID = e.ID
			break
		}
	}
	if guestID == "" {
		t.Logf("external 'guest' network not found (have %d external nets) — skipping router/gateway", len(exts))
		return
	}

	// 4. router with external gateway = guest
	r, err := c.CreateRouter(ctx, CreateRouterOpts{Name: "stratos-go-write-smoke-rtr", ExternalGatewayNetworkID: guestID})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	rID, _ := r["id"].(string)
	if rID == "" {
		t.Fatalf("create router: no id in %v", r)
	}
	t.Cleanup(func() {
		if err := c.DeleteRouter(context.Background(), rID); err != nil {
			t.Errorf("cleanup router %s: %v", rID, err)
		} else {
			t.Logf("deleted router %s", rID)
		}
	})
	t.Logf("created router %s (gw=guest %s)", rID, guestID)

	// 5. attach the subnet to the router
	if err := c.AddRouterInterface(ctx, rID, snID); err != nil {
		t.Fatalf("add router interface: %v", err)
	}
	t.Cleanup(func() {
		if err := c.RemoveRouterInterface(context.Background(), rID, snID); err != nil {
			t.Errorf("cleanup router interface: %v", err)
		} else {
			t.Logf("removed router interface %s/%s", rID, snID)
		}
	})
	t.Logf("attached subnet %s to router %s — full internal VPC up", snID, rID)
}

func writeSmokeClient(t *testing.T) (*Client, context.Context) {
	t.Helper()
	if os.Getenv("OS_AUTH_URL") == "" || os.Getenv("STRATOS_WRITE_SMOKE") != "1" {
		t.Skip("OS_AUTH_URL + STRATOS_WRITE_SMOKE=1 required — skipping live write smoke")
	}
	cfg := Config{
		AuthURL: os.Getenv("OS_AUTH_URL"), Region: os.Getenv("OS_REGION_NAME"),
		Username: os.Getenv("OS_USERNAME"), Password: os.Getenv("OS_PASSWORD"),
		UserDomainName: os.Getenv("OS_USER_DOMAIN_NAME"),
		ProjectName:    os.Getenv("OS_PROJECT_NAME"), ProjectDomainName: os.Getenv("OS_PROJECT_DOMAIN_NAME"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return c, ctx
}

// TestWriteSmokeVolume: live create→verify→delete of a Cinder volume.
func TestWriteSmokeVolume(t *testing.T) {
	c, ctx := writeSmokeClient(t)
	vol, err := c.CreateVolume(ctx, CreateVolumeOpts{Name: "stratos-go-vol-smoke", Size: 1})
	if err != nil {
		t.Fatalf("create volume: %v", err)
	}
	volID, _ := vol["id"].(string)
	if volID == "" {
		t.Fatalf("create volume: no id in %v", vol)
	}
	t.Logf("created volume %s", volID)
	t.Cleanup(func() {
		// volumes can't be deleted while "creating"; wait until available, then delete.
		for i := 0; i < 20; i++ {
			v, err := c.GetVolume(context.Background(), volID)
			if err != nil {
				break
			}
			if s, _ := v["status"].(string); s == "available" || s == "error" {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if err := c.DeleteVolume(context.Background(), volID); err != nil {
			t.Errorf("cleanup volume %s: %v", volID, err)
		} else {
			t.Logf("deleted volume %s", volID)
		}
	})
}

// TestWriteSmokePortFip: live create→associate→cleanup of a Neutron port + floating IP on a
// fresh internal net (port on the net; FIP allocated from the external "guest" + associated).
func TestWriteSmokePortFip(t *testing.T) {
	c, ctx := writeSmokeClient(t)
	dhcp := true
	net, err := c.CreateNetwork(ctx, CreateNetworkOpts{Name: "stratos-go-pf-net"})
	if err != nil {
		t.Fatalf("create net: %v", err)
	}
	netID, _ := net["id"].(string)
	t.Cleanup(func() { _ = c.DeleteNetwork(context.Background(), netID); t.Logf("deleted net %s", netID) })
	sn, err := c.CreateSubnet(ctx, CreateSubnetOpts{NetworkID: netID, Name: "stratos-go-pf-sn", CIDR: "192.168.238.0/24", IPVersion: 4, EnableDHCP: &dhcp, Gateway: true})
	if err != nil {
		t.Fatalf("create subnet: %v", err)
	}
	snID, _ := sn["id"].(string)
	t.Cleanup(func() { _ = c.DeleteSubnet(context.Background(), snID) })

	// find guest external net up front — needed for both the router gateway and the FIP pool.
	exts, _ := c.ListExternalNetworks(ctx)
	guestID := ""
	for _, e := range exts {
		if e.Name == "guest" {
			guestID = e.ID
			break
		}
	}
	if guestID == "" {
		t.Logf("no guest external net — skipping port/fip smoke")
		return
	}
	// router(gw=guest) + subnet interface → makes the subnet reachable so a FIP can associate.
	r, err := c.CreateRouter(ctx, CreateRouterOpts{Name: "stratos-go-pf-rtr", ExternalGatewayNetworkID: guestID})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	rID, _ := r["id"].(string)
	t.Cleanup(func() { _ = c.DeleteRouter(context.Background(), rID) })
	if err := c.AddRouterInterface(ctx, rID, snID); err != nil {
		t.Fatalf("add router interface: %v", err)
	}
	t.Cleanup(func() { _ = c.RemoveRouterInterface(context.Background(), rID, snID) })

	port, err := c.CreatePort(ctx, CreatePortOpts{NetworkID: netID, Name: "stratos-go-pf-port"})
	if err != nil {
		t.Fatalf("create port: %v", err)
	}
	portID, _ := port["id"].(string)
	if portID == "" {
		t.Fatalf("create port: no id in %v", port)
	}
	t.Cleanup(func() {
		if err := c.DeletePort(context.Background(), portID); err != nil {
			t.Errorf("cleanup port: %v", err)
		} else {
			t.Logf("deleted port %s", portID)
		}
	})
	t.Logf("created port %s", portID)

	fip, err := c.CreateFloatingIP(ctx, CreateFloatingIPOpts{FloatingNetworkID: guestID})
	if err != nil {
		t.Fatalf("create fip: %v", err)
	}
	fipID, _ := fip["id"].(string)
	if fipID == "" {
		t.Fatalf("create fip: no id in %v", fip)
	}
	t.Cleanup(func() {
		if err := c.DeleteFloatingIP(context.Background(), fipID); err != nil {
			t.Errorf("cleanup fip: %v", err)
		} else {
			t.Logf("deleted fip %s", fipID)
		}
	})
	t.Logf("created fip %s (from guest)", fipID)

	if _, err := c.AssociateFloatingIP(ctx, fipID, portID); err != nil {
		t.Fatalf("associate fip→port: %v", err)
	}
	t.Logf("associated fip %s → port %s", fipID, portID)
	if _, err := c.DisassociateFloatingIP(ctx, fipID); err != nil {
		t.Fatalf("disassociate fip: %v", err)
	}
	t.Logf("disassociated fip %s — port+fip path OK", fipID)
}

// TestWriteSmokeServer: live boot→verify→delete of a Nova VM on a freshly-created internal net.
// The VM is deleted and polled gone before the subnet/network teardown (port-in-use guard).
func TestWriteSmokeServer(t *testing.T) {
	c, ctx := writeSmokeClient(t)

	// smallest usable flavor (disk>0, ram>0) + first active image
	flavors, err := c.ListFlavors(ctx)
	if err != nil {
		t.Fatalf("list flavors: %v", err)
	}
	var fl *Flavor
	for i := range flavors {
		f := flavors[i]
		if f.Disk > 0 && f.RAM > 0 && (fl == nil || f.RAM < fl.RAM) {
			fl = &flavors[i]
		}
	}
	if fl == nil {
		t.Skip("no flavor with disk>0 — skipping VM smoke")
	}
	imgs, err := c.ListImages(ctx)
	if err != nil {
		t.Fatalf("list images: %v", err)
	}
	imgID := ""
	for _, im := range imgs {
		if im.Status == "active" {
			imgID = im.ID
			break
		}
	}
	if imgID == "" {
		t.Skip("no active image — skipping VM smoke")
	}

	// internal net + subnet for the VM
	net, err := c.CreateNetwork(ctx, CreateNetworkOpts{Name: "stratos-go-vm-smoke-net"})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	netID, _ := net["id"].(string)
	t.Cleanup(func() {
		if err := c.DeleteNetwork(context.Background(), netID); err != nil {
			t.Errorf("cleanup network %s: %v", netID, err)
		}
	})
	dhcp := true
	sn, err := c.CreateSubnet(ctx, CreateSubnetOpts{NetworkID: netID, Name: "stratos-go-vm-smoke-sn", CIDR: "192.168.235.0/24", IPVersion: 4, EnableDHCP: &dhcp, Gateway: true})
	if err != nil {
		t.Fatalf("create subnet: %v", err)
	}
	snID, _ := sn["id"].(string)
	t.Cleanup(func() {
		if err := c.DeleteSubnet(context.Background(), snID); err != nil {
			t.Errorf("cleanup subnet %s: %v", snID, err)
		}
	})

	// boot the VM (status BUILD immediately; we don't wait for ACTIVE)
	srv, err := c.CreateServer(ctx, CreateServerOpts{
		Name: "stratos-go-vm-smoke", FlavorID: fl.ID, ImageID: imgID, NetworkIDs: []string{netID},
	})
	if err != nil {
		t.Fatalf("create server (flavor=%s image=%s): %v", fl.Name, imgID, err)
	}
	srvID, _ := srv["id"].(string)
	if srvID == "" {
		t.Fatalf("create server: no id in %v", srv)
	}
	t.Logf("booted VM %s (flavor=%s image=%s)", srvID, fl.Name, imgID)
	// delete VM + poll gone BEFORE the subnet/net teardown so the port is released first.
	t.Cleanup(func() {
		if err := c.DeleteServer(context.Background(), srvID); err != nil {
			t.Errorf("cleanup server %s: %v", srvID, err)
			return
		}
		for i := 0; i < 30; i++ {
			if _, err := c.GetServer(context.Background(), srvID); err != nil {
				t.Logf("deleted VM %s", srvID)
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Logf("VM %s delete issued (still terminating)", srvID)
	})
}
