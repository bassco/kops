package cloudup

import (
	"fmt"
	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"strings"
	"testing"
)

func TestDeepValidate_OK(t *testing.T) {
	c := buildDefaultCluster(t)
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a"))
	err := api.DeepValidate(c, groups, true)
	if err != nil {
		t.Fatalf("Expected no error from DeepValidate")
	}
}

func TestDeepValidate_NoNodeZones(t *testing.T) {
	c := buildDefaultCluster(t)
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a"))
	expectErrorFromDeepValidate(t, c, groups, "must configure at least one Node InstanceGroup")
}

func TestDeepValidate_NoMasterZones(t *testing.T) {
	c := buildDefaultCluster(t)
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a"))
	expectErrorFromDeepValidate(t, c, groups, "must configure at least one Master InstanceGroup")
}

func TestDeepValidate_BadZone(t *testing.T) {
	t.Skipf("Zone validation not checked by DeepValidate")
	c := buildDefaultCluster(t)
	c.Spec.Zones = []*api.ClusterZoneSpec{
		{Name: "us-mock-1z", CIDR: "172.20.1.0/24"},
	}
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1z"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1z"))
	expectErrorFromDeepValidate(t, c, groups, "Zone is not a recognized AZ")
}

func TestDeepValidate_MixedRegion(t *testing.T) {
	t.Skipf("Region validation not checked by DeepValidate")
	c := buildDefaultCluster(t)
	c.Spec.Zones = []*api.ClusterZoneSpec{
		{Name: "us-mock-1a", CIDR: "172.20.1.0/24"},
		{Name: "us-west-1b", CIDR: "172.20.2.0/24"},
	}
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a", "us-west-1b"))

	expectErrorFromDeepValidate(t, c, groups, "Clusters cannot span multiple regions")
}

func TestDeepValidate_RegionAsZone(t *testing.T) {
	t.Skipf("Region validation not checked by DeepValidate")
	c := buildDefaultCluster(t)
	c.Spec.Zones = []*api.ClusterZoneSpec{
		{Name: "us-mock-1", CIDR: "172.20.1.0/24"},
	}
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1"))

	expectErrorFromDeepValidate(t, c, groups, "Region is not a recognized EC2 region: \"us-east-\" (check you have specified valid zones?)")
}

func TestDeepValidate_NotIncludedZone(t *testing.T) {
	c := buildDefaultCluster(t)
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1d"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1d"))

	expectErrorFromDeepValidate(t, c, groups, "not configured as a Zone in the cluster")
}

func TestDeepValidate_DuplicateZones(t *testing.T) {
	c := buildDefaultCluster(t)
	c.Spec.Zones = []*api.ClusterZoneSpec{
		{Name: "us-mock-1a", CIDR: "172.20.1.0/24"},
		{Name: "us-mock-1a", CIDR: "172.20.2.0/24"},
	}
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a"))
	expectErrorFromDeepValidate(t, c, groups, "Zones contained a duplicate value: us-mock-1a")
}

func TestDeepValidate_ExtraMasterZone(t *testing.T) {
	c := buildDefaultCluster(t)
	c.Spec.Zones = []*api.ClusterZoneSpec{
		{Name: "us-mock-1a", CIDR: "172.20.1.0/24"},
		{Name: "us-mock-1b", CIDR: "172.20.2.0/24"},
	}
	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a", "us-mock-1b", "us-mock-1c"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a", "us-mock-1b"))

	expectErrorFromDeepValidate(t, c, groups, "is not configured as a Zone in the cluster")
}

func TestDeepValidate_EvenEtcdClusterSize(t *testing.T) {
	c := buildDefaultCluster(t)
	c.Spec.EtcdClusters = []*api.EtcdClusterSpec{
		{
			Name: "main",
			Members: []*api.EtcdMemberSpec{
				{Name: "us-mock-1a", Zone: fi.String("us-mock-1a")},
				{Name: "us-mock-1b", Zone: fi.String("us-mock-1b")},
			},
		},
	}

	var groups []*api.InstanceGroup
	groups = append(groups, buildMinimalMasterInstanceGroup("us-mock-1a", "us-mock-1b", "us-mock-1c", "us-mock-1d"))
	groups = append(groups, buildMinimalNodeInstanceGroup("us-mock-1a"))

	expectErrorFromDeepValidate(t, c, groups, "There should be an odd number of master-zones, for etcd's quorum.  Hint: Use --zones and --master-zones to declare node zones and master zones separately.")
}

func expectErrorFromDeepValidate(t *testing.T, c *api.Cluster, groups []*api.InstanceGroup, message string) {
	err := api.DeepValidate(c, groups, true)
	if err == nil {
		t.Fatalf("Expected error from DeepValidate (strict=true)")
	}
	actualMessage := fmt.Sprintf("%v", err)
	if !strings.Contains(actualMessage, message) {
		t.Fatalf("Expected error %q, got %q", message, actualMessage)
	}
}
