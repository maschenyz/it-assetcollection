package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type SoftwareEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Publisher   string `json:"publisher"`
	InstallPath string `json:"install_path"`
}

// GetSoftwareList scans installed programs from registry + custom HOSxP detection
func GetSoftwareList() []SoftwareEntry {
	seen := map[string]bool{}
	var list []SoftwareEntry

	// 1. Custom detection: HOSxP XE4 via AppData scan
	if usersDir := os.Getenv("SystemDrive"); usersDir != "" {
		pattern := filepath.Join(usersDir+"\\Users", "*", "AppData", "Roaming", "BMS", "HOSxPXE4")
		matches, _ := filepath.Glob(pattern)
		for _, p := range matches {
			info, err := os.Stat(p)
			if err == nil && info.IsDir() {
				key := "hosxp xe4"
				if !seen[key] {
					seen[key] = true
					list = append(list, SoftwareEntry{
						Name:        "HOSxP XE4",
						Version:     "N/A",
						Publisher:   "BMS (Bangkok Medical Software)",
						InstallPath: p,
					})
				}
			}
		}
	}

	// 2. Registry scan from all standard Uninstall paths
	paths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}

	for _, p := range paths {
		k, err := registry.OpenKey(p.root, p.path, registry.READ)
		if err != nil {
			continue
		}
		subkeys, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			continue
		}

		for _, sub := range subkeys {
			subPath := p.path + `\` + sub
			sk, err := registry.OpenKey(p.root, subPath, registry.READ)
			if err != nil {
				continue
			}

			name, _, _ := sk.GetStringValue("DisplayName")
			version, _, _ := sk.GetStringValue("DisplayVersion")
			publisher, _, _ := sk.GetStringValue("Publisher")
			installPath, _, _ := sk.GetStringValue("InstallLocation")
			sk.Close()

			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			version = strings.TrimSpace(version)
			publisher = strings.TrimSpace(publisher)
			installPath = strings.TrimSpace(installPath)
			if version == "" {
				version = "N/A"
			}
			if installPath == "" {
				installPath = "N/A"
			}

			dupKey := strings.ToLower(fmt.Sprintf("%s|%s|%s", name, version, publisher))
			if seen[dupKey] {
				continue
			}
			seen[dupKey] = true
			list = append(list, SoftwareEntry{
				Name:        name,
				Version:     version,
				Publisher:   publisher,
				InstallPath: installPath,
			})
		}
	}

	return list
}
