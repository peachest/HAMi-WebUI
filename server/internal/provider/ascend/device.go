package ascend

import "vgpu/internal/provider/util"

const (
	AscendDevice          = "Ascend" // 保留: base-charts commonWord for 910B3
	Ascend310PDevice      = "Ascend310P"
	Ascend910ADevice      = "Ascend910A"
	Ascend910B2Device     = "Ascend910B2"
	Ascend910B3Device     = "Ascend910B3"
	Ascend910B4Device     = "Ascend910B4"
	Ascend910B4_1Device   = "Ascend910B4-1"
	Ascend910CDevice      = "Ascend910C"
	AscendDeviceSelection = "huawei.com/predicate-ascend-idx-"
	// IluvatarUseUUID is user can use specify Iluvatar device for set Iluvatar UUID.
	AscendDeviceUseUUID = "huawei.com/use-ascenduuid"
	// IluvatarNoUseUUID is user can not use specify Iluvatar device for set Iluvatar UUID.
	AscendNoUseUUID             = "huawei.com/nouse-ascenduuid"
	Ascend910ANodeRegisterAnno  = "hami.io/node-register-Ascend910A"
	Ascend910BNodeRegisterAnno  = "hami.io/node-register-Ascend910B"
	Ascend910B2NodeRegisterAnno = "hami.io/node-register-Ascend910B2"
	Ascend910B3NodeRegisterAnno = "hami.io/node-register-Ascend910B3"
	Ascend910B4NodeRegisterAnno = "hami.io/node-register-Ascend910B4"
	Ascend910B4_1NodeRegisterAnno = "hami.io/node-register-Ascend910B4-1"
	Ascend910CNodeRegisterAnno  = "hami.io/node-register-Ascend910C"
	Ascend310PNodeRegisterAnno  = "hami.io/node-register-Ascend310P"

	AscendNodeRegisterAnnoPrefix = "hami.io/node-register-Ascend"

	// Ascend910CDeviceMergeSeparator 用于拼接双芯片 UUID
	Ascend910CDeviceMergeSeparator = "---"
)

var (
	AscendResourceCount     string
	AscendResourceMemory    string
	AscendResourceCores     string
	AscendNodeRegisterAnnos []string
)

func init() {
	AscendNodeRegisterAnnos = []string{
		Ascend910ANodeRegisterAnno,
		Ascend910BNodeRegisterAnno,
		Ascend910B2NodeRegisterAnno,
		Ascend910B3NodeRegisterAnno,
		Ascend910B4NodeRegisterAnno,
		Ascend910B4_1NodeRegisterAnno,
		Ascend910CNodeRegisterAnno,
		Ascend310PNodeRegisterAnno,
	}

	register := func(devType string) {
		util.InRequestDevices[devType] = "hami.io/" + devType + "-devices-to-allocate"
		util.SupportDevices[devType] = "hami.io/" + devType + "-devices-allocated"
	}

	// Ascend910B (base-charts commonWord for 910B3) — 保留 "Ascend" 作为 devType 以兼容现有逻辑
	util.InRequestDevices[AscendDevice] = "hami.io/Ascend910B-devices-to-allocate"
	util.SupportDevices[AscendDevice] = "hami.io/Ascend910B-devices-allocated"
	// Ascend910B3 (HAMi chart commonWord for 910B3)
	register(Ascend910B3Device)
	// 其余型号
	register(Ascend910ADevice)
	register(Ascend910B2Device)
	register(Ascend910B4Device)
	register(Ascend910B4_1Device)
	register(Ascend910CDevice)
	register(Ascend310PDevice)
}
