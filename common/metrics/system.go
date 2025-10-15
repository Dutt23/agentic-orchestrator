package metrics

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// captureSystemInfo gathers comprehensive system information
func captureSystemInfo() *SystemInfo {
	info := &SystemInfo{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPULogical: runtime.NumCPU(),
		GoVersion:  runtime.Version(),
	}

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	} else {
		info.Hostname = "unknown"
	}

	// Detect container environment
	info.InContainer, info.ContainerRuntime = detectContainer()

	// Get OS version and physical CPU cores (platform-specific)
	info.OSVersion = getOSVersion()
	info.CPUCores = getPhysicalCores()
	info.TotalMemoryMB = getTotalMemory()

	return info
}

// detectContainer checks if running in a container
func detectContainer() (bool, string) {
	// Check for Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true, "docker"
	}

	// Check for Kubernetes
	if _, err := os.Stat("/var/run/secrets/kubernetes.io"); err == nil {
		return true, "kubernetes"
	}

	// Check cgroup for container indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") {
			return true, "docker"
		}
		if strings.Contains(content, "kubepods") {
			return true, "kubernetes"
		}
		if strings.Contains(content, "containerd") {
			return true, "containerd"
		}
	}

	return false, ""
}

// getOSVersion returns the OS version string
func getOSVersion() string {
	switch runtime.GOOS {
	case "linux":
		return getLinuxVersion()
	case "darwin":
		return getMacOSVersion()
	case "windows":
		return getWindowsVersion()
	default:
		return "unknown"
	}
}

// getLinuxVersion gets Linux distribution and version
func getLinuxVersion() string {
	// Try /etc/os-release first (most modern distros)
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		var name, version string
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			}
			if strings.HasPrefix(line, "NAME=") {
				name = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
			}
			if strings.HasPrefix(line, "VERSION=") {
				version = strings.Trim(strings.TrimPrefix(line, "VERSION="), "\"")
			}
		}
		if name != "" && version != "" {
			return name + " " + version
		}
		if name != "" {
			return name
		}
	}

	// Fallback to uname
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		return "Linux " + strings.TrimSpace(string(out))
	}

	return "Linux (unknown)"
}

// getMacOSVersion gets macOS version
func getMacOSVersion() string {
	// Try sw_vers command
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		version := strings.TrimSpace(string(out))
		if name, err := exec.Command("sw_vers", "-productName").Output(); err == nil {
			return strings.TrimSpace(string(name)) + " " + version
		}
		return "macOS " + version
	}

	// Fallback to uname
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		return "macOS " + strings.TrimSpace(string(out))
	}

	return "macOS (unknown)"
}

// getWindowsVersion gets Windows version
func getWindowsVersion() string {
	// Try ver command
	if out, err := exec.Command("cmd", "/c", "ver").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}

	// Try wmic for more details
	if out, err := exec.Command("wmic", "os", "get", "Caption,Version", "/value").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		var caption, version string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Caption=") {
				caption = strings.TrimPrefix(line, "Caption=")
			}
			if strings.HasPrefix(line, "Version=") {
				version = strings.TrimPrefix(line, "Version=")
			}
		}
		if caption != "" && version != "" {
			return caption + " (Version " + version + ")"
		}
		if caption != "" {
			return caption
		}
	}

	return "Windows (unknown)"
}

// getPhysicalCores attempts to get physical CPU core count
func getPhysicalCores() int {
	switch runtime.GOOS {
	case "linux":
		return getLinuxPhysicalCores()
	case "darwin":
		return getMacOSPhysicalCores()
	case "windows":
		return getWindowsPhysicalCores()
	default:
		// Fallback: assume no hyperthreading
		return runtime.NumCPU()
	}
}

// getLinuxPhysicalCores gets physical cores on Linux
func getLinuxPhysicalCores() int {
	// Try lscpu command
	if out, err := exec.Command("lscpu", "-p=Core").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		coreSet := make(map[string]bool)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				coreSet[line] = true
			}
		}
		if len(coreSet) > 0 {
			return len(coreSet)
		}
	}

	// Fallback to /proc/cpuinfo
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		coreIDs := make(map[string]bool)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "core id") {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					coreIDs[strings.TrimSpace(parts[1])] = true
				}
			}
		}
		if len(coreIDs) > 0 {
			return len(coreIDs)
		}
	}

	// Fallback
	return runtime.NumCPU()
}

// getMacOSPhysicalCores gets physical cores on macOS
func getMacOSPhysicalCores() int {
	if out, err := exec.Command("sysctl", "-n", "hw.physicalcpu").Output(); err == nil {
		var cores int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &cores); err == nil && cores > 0 {
			return cores
		}
	}
	return runtime.NumCPU()
}

// getWindowsPhysicalCores gets physical cores on Windows
func getWindowsPhysicalCores() int {
	if out, err := exec.Command("wmic", "cpu", "get", "NumberOfCores", "/value").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "NumberOfCores=") {
				var cores int
				if _, err := fmt.Sscanf(strings.TrimPrefix(line, "NumberOfCores="), "%d", &cores); err == nil && cores > 0 {
					return cores
				}
			}
		}
	}
	return runtime.NumCPU()
}

// getTotalMemory gets total system memory in MB
func getTotalMemory() uint64 {
	switch runtime.GOOS {
	case "linux":
		return getLinuxMemory()
	case "darwin":
		return getMacOSMemory()
	case "windows":
		return getWindowsMemory()
	default:
		return 0
	}
}

// getLinuxMemory gets total memory on Linux
func getLinuxMemory() uint64 {
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					var memKB uint64
					if _, err := fmt.Sscanf(fields[1], "%d", &memKB); err == nil {
						return memKB / 1024 // Convert KB to MB
					}
				}
			}
		}
	}
	return 0
}

// getMacOSMemory gets total memory on macOS
func getMacOSMemory() uint64 {
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		var memBytes uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &memBytes); err == nil {
			return memBytes / 1024 / 1024 // Convert bytes to MB
		}
	}
	return 0
}

// getWindowsMemory gets total memory on Windows
func getWindowsMemory() uint64 {
	if out, err := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "TotalPhysicalMemory=") {
				var memBytes uint64
				if _, err := fmt.Sscanf(strings.TrimPrefix(line, "TotalPhysicalMemory="), "%d", &memBytes); err == nil {
					return memBytes / 1024 / 1024 // Convert bytes to MB
				}
			}
		}
	}
	return 0
}
