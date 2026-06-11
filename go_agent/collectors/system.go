package collectors

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// SystemInfo holds OS-level telemetry
type SystemInfo struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Caption      string `json:"caption"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	WindowsBuild string `json:"windows_build"`
	BiosSerial   string `json:"bios_serial"`
	LastUser     string `json:"last_user"`
}

func GetSystemInfo() SystemInfo {
	info := SystemInfo{
		OS:           "Windows",
		Architecture: "x86_64",
	}

	host, _ := os.Hostname()
	info.Hostname = host

	// Try current logged-in user
	if u := os.Getenv("USERNAME"); u != "" {
		info.LastUser = u
	}

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// already initialized
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return info
	}
	defer unknown.Release()
	wmi, _ := unknown.QueryInterface(ole.IID_IDispatch)
	defer wmi.Release()
	serviceRaw, _ := oleutil.CallMethod(wmi, "ConnectServer")
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	// OS Info (Caption, BuildNumber, Version)
	if res, err := oleutil.CallMethod(service, "ExecQuery",
		"SELECT Caption, BuildNumber, Version FROM Win32_OperatingSystem"); err == nil {
		r := res.ToIDispatch()
		defer r.Release()
		if item, err := oleutil.CallMethod(r, "ItemIndex", 0); err == nil {
			it := item.ToIDispatch()
			defer it.Release()
			if v, err := oleutil.GetProperty(it, "Caption"); err == nil {
				info.Caption = strings.TrimSpace(fmt.Sprintf("%v", v.Value()))
			}
			if v, err := oleutil.GetProperty(it, "BuildNumber"); err == nil {
				info.WindowsBuild = strings.TrimSpace(fmt.Sprintf("%v", v.Value()))
			}
			if v, err := oleutil.GetProperty(it, "Version"); err == nil {
				info.Version = strings.TrimSpace(fmt.Sprintf("%v", v.Value()))
			}
		}
	}

	// BIOS Serial
	if res, err := oleutil.CallMethod(service, "ExecQuery",
		"SELECT SerialNumber FROM Win32_BIOS"); err == nil {
		r := res.ToIDispatch()
		defer r.Release()
		if item, err := oleutil.CallMethod(r, "ItemIndex", 0); err == nil {
			it := item.ToIDispatch()
			defer it.Release()
			if v, err := oleutil.GetProperty(it, "SerialNumber"); err == nil {
				info.BiosSerial = strings.TrimSpace(fmt.Sprintf("%v", v.Value()))
			}
		}
	}

	// Logged on user via quser (best effort)
	if out, err := exec.Command("quser").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			parts := strings.Fields(lines[1])
			if len(parts) > 0 {
				info.LastUser = strings.TrimLeft(parts[0], ">")
			}
		}
	}

	return info
}

// GetNetworkInfo holds active IP, MAC, connection type, WiFi SSID
type NetworkInfo struct {
	IP   string `json:"ip"`
	MAC  string `json:"mac"`
	Type string `json:"type"` // LAN or Wi-Fi
}

func GetNetworkInfo() NetworkInfo {
	info := NetworkInfo{Type: "LAN"}

	// Get IP/MAC from WMI (NetworkAdapterConfiguration where IPEnabled=TRUE)
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// ok
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return info
	}
	defer unknown.Release()
	wmi, _ := unknown.QueryInterface(ole.IID_IDispatch)
	defer wmi.Release()
	serviceRaw, _ := oleutil.CallMethod(wmi, "ConnectServer")
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	res, err := oleutil.CallMethod(service, "ExecQuery",
		"SELECT IPAddress, MACAddress, Description FROM Win32_NetworkAdapterConfiguration WHERE IPEnabled=TRUE")
	if err == nil {
		r := res.ToIDispatch()
		defer r.Release()
		countVar, _ := oleutil.GetProperty(r, "Count")
		count := int(countVar.Val)
		for i := 0; i < count; i++ {
			itemRaw, err := oleutil.CallMethod(r, "ItemIndex", i)
			if err != nil {
				continue
			}
			it := itemRaw.ToIDispatch()

			macV, _ := oleutil.GetProperty(it, "MACAddress")
			mac := strings.TrimSpace(fmt.Sprintf("%v", macV.Value()))

			descV, _ := oleutil.GetProperty(it, "Description")
			desc := strings.ToLower(fmt.Sprintf("%v", descV.Value()))

			ipV, _ := oleutil.GetProperty(it, "IPAddress")
			// IPAddress is a SAFEARRAY — we parse it as JSON
			ipRaw, _ := json.Marshal(ipV.Value())
			var ips []string
			_ = json.Unmarshal(ipRaw, &ips)

			it.Release()

			for _, ip := range ips {
				if strings.Contains(ip, ".") && !strings.HasPrefix(ip, "127.") {
					info.IP = ip
					info.MAC = mac
					if strings.Contains(desc, "wireless") || strings.Contains(desc, "wi-fi") ||
						strings.Contains(desc, "wlan") || strings.Contains(desc, "802.11") {
						info.Type = "Wi-Fi"
					}
					goto done
				}
			}
		}
	}
done:
	return info
}

func GetWiFiSSID() string {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) == "SSID" {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func GetSystemStats() map[string]interface{} {
	// Use wmic for a quick CPU/RAM percentage
	cpuPct := 0.0
	ramPct := 0.0
	diskPct := 0.0

	// CPU load from Win32_Processor
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// ok
	}
	defer ole.CoUninitialize()

	unknown, _ := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if unknown != nil {
		defer unknown.Release()
		wmi, _ := unknown.QueryInterface(ole.IID_IDispatch)
		if wmi != nil {
			defer wmi.Release()
			serviceRaw, _ := oleutil.CallMethod(wmi, "ConnectServer")
			service := serviceRaw.ToIDispatch()
			defer service.Release()

			// CPU Load
			time.Sleep(500 * time.Millisecond) // brief pause for load measurement
			if res, err := oleutil.CallMethod(service, "ExecQuery",
				"SELECT LoadPercentage FROM Win32_Processor"); err == nil {
				r := res.ToIDispatch()
				defer r.Release()
				if item, err := oleutil.CallMethod(r, "ItemIndex", 0); err == nil {
					it := item.ToIDispatch()
					defer it.Release()
					if v, err := oleutil.GetProperty(it, "LoadPercentage"); err == nil {
						cpuPct = float64(v.Val)
					}
				}
			}

			// RAM: FreePhysicalMemory vs TotalVisibleMemorySize
			if res, err := oleutil.CallMethod(service, "ExecQuery",
				"SELECT FreePhysicalMemory, TotalVisibleMemorySize FROM Win32_OperatingSystem"); err == nil {
				r := res.ToIDispatch()
				defer r.Release()
				if item, err := oleutil.CallMethod(r, "ItemIndex", 0); err == nil {
					it := item.ToIDispatch()
					defer it.Release()
					freeV, _ := oleutil.GetProperty(it, "FreePhysicalMemory")
					totalV, _ := oleutil.GetProperty(it, "TotalVisibleMemorySize")
					free := float64(freeV.Val)
					total := float64(totalV.Val)
					if total > 0 {
						ramPct = ((total - free) / total) * 100.0
					}
				}
			}

			// Disk usage C:
			if res, err := oleutil.CallMethod(service, "ExecQuery",
				`SELECT FreeSpace, Size FROM Win32_LogicalDisk WHERE DeviceID='C:'`); err == nil {
				r := res.ToIDispatch()
				defer r.Release()
				if item, err := oleutil.CallMethod(r, "ItemIndex", 0); err == nil {
					it := item.ToIDispatch()
					defer it.Release()
					freeV, _ := oleutil.GetProperty(it, "FreeSpace")
					sizeV, _ := oleutil.GetProperty(it, "Size")
					free := float64(freeV.Val)
					size := float64(sizeV.Val)
					if size > 0 {
						diskPct = ((size - free) / size) * 100.0
					}
				}
			}
		}
	}

	return map[string]interface{}{
		"cpu_pct":  cpuPct,
		"ram_pct":  ramPct,
		"disk_pct": diskPct,
	}
}
