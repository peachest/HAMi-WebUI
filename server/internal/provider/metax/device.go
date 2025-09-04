package metax

import "vgpu/internal/provider/util"

const (
	RegisterAnnos       = "hami.io/node-dcu-register"
	MetaxSGPUDevice     = "Metax-SGPU"
	MetaxSGPUCommonWord = "Metax-SGPU"
)

func init() {
	util.InRequestDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-to-allocate"
	util.SupportDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-allocated"
}
