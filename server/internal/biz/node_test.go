package biz

import "testing"

func TestDeviceInfo_MatchAlias_MergedDevice(t *testing.T) {
	d := &DeviceInfo{AliasId: "6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3---6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3"}

	// chip0's container UUID should match
	if !d.MatchAlias("6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3") {
		t.Errorf("MatchAlias should match chip0 UUID")
	}

	// chip1's container UUID should match
	if !d.MatchAlias("6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3") {
		t.Errorf("MatchAlias should match chip1 UUID")
	}

	// unrelated UUID should NOT match
	if d.MatchAlias("00000000-0000-0000-0000-000000000000") {
		t.Errorf("MatchAlias should not match unrelated UUID")
	}
}

func TestDeviceInfo_MatchAlias_NonMergedDevice(t *testing.T) {
	d := &DeviceInfo{AliasId: "6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3"}

	if !d.MatchAlias("6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3") {
		t.Errorf("MatchAlias should match exact UUID for non-merged device")
	}

	if !d.MatchAlias("6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3") {
		t.Errorf("MatchAlias should match exact UUID")
	}

	if d.MatchAlias("") {
		t.Errorf("MatchAlias should not match empty string")
	}
}

func TestDeviceInfo_ExactAlias_MergedDevice(t *testing.T) {
	d := &DeviceInfo{AliasId: "6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3---6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3"}

	// chip0's exact UUID should match
	if !d.ExactAlias("6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3") {
		t.Errorf("ExactAlias should match chip0 UUID exactly")
	}

	// chip1's exact UUID should match
	if !d.ExactAlias("6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3") {
		t.Errorf("ExactAlias should match chip1 UUID exactly")
	}

	// a prefix of chip0 UUID should NOT match (ExactAlias is exact)
	if d.ExactAlias("6ECA2A6C-0100F7B1") {
		t.Errorf("ExactAlias should not match prefix")
	}
}

func TestDeviceInfo_ExactAlias_MergedDevice_FullAliasId(t *testing.T) {
	d := &DeviceInfo{AliasId: "6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3---6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3"}

	// the full combined AliasId should also match (consumer may use whole merged ID)
	if !d.ExactAlias("6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3---6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3") {
		t.Errorf("ExactAlias should match the full combined AliasId")
	}
}
