package metax

import "vgpu/internal/provider/util"

const (
	RegisterAnnos       = "metax-tech.com/node-gpu-devices"
	MetaxSGPUDevice     = "Metax-SGPU"
	MetaxSGPUCommonWord = "Metax-SGPU"
)

func init() {
	util.InRequestDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-to-allocate"
	util.SupportDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-allocated"
}
