package ppu

import (
	"vgpu/internal/biz"
	"vgpu/internal/provider/util"
)

const (
	HandshakeAnnos   = "hami.io/node-handshake-ppu"
	RegisterAnnos    = "hami.io/node-ppu-register"
	PPUAllocatedAnno = "hami.io/ppu-devices-allocated"
	PPURequestAnno   = "hami.io/ppu-devices-to-allocate"
)

func init() {
	util.InRequestDevices[biz.AlibabaPPUDevice] = PPURequestAnno
	util.SupportDevices[biz.AlibabaPPUDevice] = PPUAllocatedAnno
}
