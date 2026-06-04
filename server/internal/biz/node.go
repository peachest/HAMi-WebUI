package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
)

type Node struct {
	Name                    string
	IP                      string
	IsSchedulable           bool
	IsReady                 bool
	Uid                     string
	OSImage                 string
	OperatingSystem         string
	KernelVersion           string
	ContainerRuntimeVersion string
	KubeletVersion          string
	KubeProxyVersion        string
	Architecture            string
	CreationTimestamp       string
	Devices                 []*DeviceInfo
}

type DeviceInfo struct {
	Index    int
	Id       string
	AliasId  string
	Count    int32
	Devmem   int32
	Devcore  int32
	Type     string
	Numa     int
	Mode     string
	Health   bool
	NodeName string
	NodeUid  string
	Provider string
	Driver   string
}

// MatchAlias 判断 cdUUID 是否属于此 device。
// 对含 "---" （合并设备）的 AliasId 拆开分别做 HasPrefix 匹配。
// 对不含 "---" 的 AliasId 保持原有 HasPrefix 行为。
func (d *DeviceInfo) MatchAlias(cdUUID string) bool {
	if !strings.Contains(d.AliasId, "---") {
		return strings.HasPrefix(cdUUID, d.AliasId)
	}
	for _, part := range strings.Split(d.AliasId, "---") {
		if strings.HasPrefix(cdUUID, part) {
			return true
		}
	}
	return false
}

// ExactAlias 精确匹配，对含 "---" 的 AliasId 拆开分别做相等判断。
// 同时也匹配完整的合并 AliasId 本身。
func (d *DeviceInfo) ExactAlias(cdUUID string) bool {
	// 非合并设备：直接比较即可
	if !strings.Contains(d.AliasId, "---") {
		return d.AliasId == cdUUID
	}
	// 合并设备：先比较完整 AliasId，再拆分比较各子 chip
	if d.AliasId == cdUUID {
		return true
	}
	for _, part := range strings.Split(d.AliasId, "---") {
		if part == cdUUID {
			return true
		}
	}
	return false
}

type DeviceTotal struct {
	VgpuCount int32
	Cores     int32
	Memory    int32
}

type NodeRepo interface {
	ListAll(context.Context) ([]*Node, error)
	GetNode(context.Context, string) (*Node, error)
	ListAllDevices(context.Context) ([]*DeviceInfo, error)
	FindDeviceByAliasId(string) (*DeviceInfo, error)
}

type NodeUsecase struct {
	repo NodeRepo
	log  *log.Helper
}

func NewNodeUsecase(repo NodeRepo, logger log.Logger) *NodeUsecase {
	return &NodeUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *NodeUsecase) ListAllNodes(ctx context.Context) ([]*Node, error) {
	return uc.repo.ListAll(ctx)
}

func (uc *NodeUsecase) GetNode(ctx context.Context, nodeName string) (*Node, error) {
	return uc.repo.GetNode(ctx, nodeName)
}

func (uc *NodeUsecase) ListAllDevices(ctx context.Context) ([]*DeviceInfo, error) {
	return uc.repo.ListAllDevices(ctx)
}

func (uc *NodeUsecase) FindDeviceByAliasId(aliasId string) (*DeviceInfo, error) {
	return uc.repo.FindDeviceByAliasId(aliasId)
}
