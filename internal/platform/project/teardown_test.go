package project

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

// TestDeletionOrder pins the dependency-safe teardown ordering: dependents delete before the things
// they reference, so a single ordered sweep rarely hits "resource still in use".
func TestDeletionOrder(t *testing.T) {
	lt := func(a, b string) {
		if deletionOrder(a) >= deletionOrder(b) {
			t.Errorf("%s (%d) should sort before %s (%d)", a, deletionOrder(a), b, deletionOrder(b))
		}
	}
	lt(cloud.TypeLoadBalancer, cloud.TypeServer) // composites before instances
	lt(cloud.TypeServer, cloud.TypeFloatingIP)   // instance releases its FIP
	lt(cloud.TypeFloatingIP, cloud.TypePort)
	lt(cloud.TypePort, cloud.TypeSubnet)
	lt(cloud.TypeSubnet, cloud.TypeNetwork)
	lt(cloud.TypeNetwork, cloud.TypeRouter)       // network before router (router removed last)
	lt(cloud.TypeVolume, cloud.TypeSubnet)        // volumes before network teardown
	lt(cloud.TypeServer, cloud.TypeSecurityGroup) // leaf types (SG/keypair/image) go last
	lt(cloud.TypeRouter, cloud.TypeSecurityGroup)
}
