package gpu

import (
	"os/exec"
)

type GPUChecker struct{}

func NewGPUChecker() *GPUChecker {
	return &GPUChecker{}
}

func (g *GPUChecker) CheckGPUAccess(containerID string) (bool, error) {
	cmd := exec.Command("docker", "exec", containerID, "nvidia-smi")
	return cmd.Run() == nil, nil
}
