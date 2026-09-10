package collectors

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// ---- WMI helpers ----

func wmiConnect() (*ole.IDispatch, *ole.IDispatch, error) {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// already initialized in this goroutine – ok
	}
	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, nil, err
	}
	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		unknown.Release()
		return nil, nil, err
	}
	unknown.Release()
	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer")
	if err != nil {
		wmi.Release()
		return nil, nil, err
	}
	return wmi, serviceRaw.ToIDispatch(), nil
}

func wmiQueryItems(service *ole.IDispatch, query string) ([]*ole.IDispatch, func()) {
	res, err := oleutil.CallMethod(service, "ExecQuery", query)
	if err != nil {
		return nil, func() {}
	}
	r := res.ToIDispatch()
	countVar, _ := oleutil.GetProperty(r, "Count")
	count := int(countVar.Val)
	items := make([]*ole.IDispatch, 0, count)
	for i := 0; i < count; i++ {
		itemRaw, err := oleutil.CallMethod(r, "ItemIndex", i)
		if err != nil {
			continue
		}
		items = append(items, itemRaw.ToIDispatch())
	}
	cleanup := func() {
		for _, it := range items {
			it.Release()
		}
		r.Release()
	}
	return items, cleanup
}

func strProp(it *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(it, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v.Value()))
}

func intProp(it *ole.IDispatch, name string) int64 {
	v, err := oleutil.GetProperty(it, name)
	if err != nil {
		return 0
	}
	s := fmt.Sprintf("%v", v.Value())
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// ---- CPU ----

type CPUInfo struct {
	Name    string `json:"name"`
	Cores   int64  `json:"cores"`
	Threads int64  `json:"threads"`
	MaxMHz  int64  `json:"max_speed"`
	L3KB    int64  `json:"l3_cache"`
}

func GetCPUInfo() CPUInfo {
	wmi, service, err := wmiConnect()
	if err != nil {
		return CPUInfo{}
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service, "SELECT Name,NumberOfCores,NumberOfLogicalProcessors,MaxClockSpeed,L3CacheSize FROM Win32_Processor")
	defer cleanup()
	if len(items) == 0 {
		return CPUInfo{}
	}
	it := items[0]
	return CPUInfo{
		Name:    strProp(it, "Name"),
		Cores:   intProp(it, "NumberOfCores"),
		Threads: intProp(it, "NumberOfLogicalProcessors"),
		MaxMHz:  intProp(it, "MaxClockSpeed"),
		L3KB:    intProp(it, "L3CacheSize"),
	}
}

// ---- RAM ----

type RAMSlot struct {
	CapacityGB   float64 `json:"capacity_gb"`
	Speed        int64   `json:"speed"`
	Manufacturer string  `json:"manufacturer"`
	PartNumber   string  `json:"part_number"`
	Serial       string  `json:"serial"`
}

func GetRAMInfo() []RAMSlot {
	wmi, service, err := wmiConnect()
	if err != nil {
		return nil
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service,
		"SELECT Capacity,Speed,Manufacturer,PartNumber,SerialNumber FROM Win32_PhysicalMemory")
	defer cleanup()

	slots := make([]RAMSlot, 0, len(items))
	for _, it := range items {
		capB, _ := strconv.ParseInt(strProp(it, "Capacity"), 10, 64)
		slots = append(slots, RAMSlot{
			CapacityGB:   math.Round(float64(capB)/(1024*1024*1024)*100) / 100,
			Speed:        intProp(it, "Speed"),
			Manufacturer: strProp(it, "Manufacturer"),
			PartNumber:   strProp(it, "PartNumber"),
			Serial:       strProp(it, "SerialNumber"),
		})
	}
	return slots
}

// ---- Storage ----

type DiskInfo struct {
	Model       string  `json:"model"`
	SizeGB      float64 `json:"size_gb"`
	Serial      string  `json:"serial"`
	Interface   string  `json:"interface"`
	MediaType   string  `json:"media_type"`
}

func GetStorageInfo() []DiskInfo {
	wmi, service, err := wmiConnect()
	if err != nil {
		return nil
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service,
		"SELECT Model,Size,SerialNumber,InterfaceType,MediaType FROM Win32_DiskDrive")
	defer cleanup()

	disks := make([]DiskInfo, 0, len(items))
	for _, it := range items {
		sizeB, _ := strconv.ParseInt(strProp(it, "Size"), 10, 64)
		disks = append(disks, DiskInfo{
			Model:     strProp(it, "Model"),
			SizeGB:    math.Round(float64(sizeB)/(1024*1024*1024)*100) / 100,
			Serial:    strProp(it, "SerialNumber"),
			Interface: strProp(it, "InterfaceType"),
			MediaType: strProp(it, "MediaType"),
		})
	}
	return disks
}

// ---- GPU ----

type GPUInfo struct {
	Name          string  `json:"name"`
	DriverVersion string  `json:"driver_version"`
	RAMMB         float64 `json:"ram_mb"`
}

func GetGPUInfo() []GPUInfo {
	wmi, service, err := wmiConnect()
	if err != nil {
		return nil
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service,
		"SELECT Name,DriverVersion,AdapterRAM FROM Win32_VideoController")
	defer cleanup()

	gpus := make([]GPUInfo, 0, len(items))
	for _, it := range items {
		ramB, _ := strconv.ParseInt(strProp(it, "AdapterRAM"), 10, 64)
		gpus = append(gpus, GPUInfo{
			Name:          strProp(it, "Name"),
			DriverVersion: strProp(it, "DriverVersion"),
			RAMMB:         math.Round(float64(ramB) / (1024 * 1024)),
		})
	}
	return gpus
}

// ---- Motherboard ----

type MotherboardInfo struct {
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Version      string `json:"version"`
}

func GetMotherboardInfo() MotherboardInfo {
	wmi, service, err := wmiConnect()
	if err != nil {
		return MotherboardInfo{}
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service,
		"SELECT Manufacturer,Product,Version FROM Win32_BaseBoard")
	defer cleanup()
	if len(items) == 0 {
		return MotherboardInfo{}
	}
	it := items[0]
	return MotherboardInfo{
		Manufacturer: strProp(it, "Manufacturer"),
		Product:      strProp(it, "Product"),
		Version:      strProp(it, "Version"),
	}
}

// ---- BIOS ----

type BIOSInfo struct {
	Version string `json:"version"`
	Serial  string `json:"serial"`
}

func GetBIOSInfo() BIOSInfo {
	wmi, service, err := wmiConnect()
	if err != nil {
		return BIOSInfo{}
	}
	defer wmi.Release()
	defer service.Release()
	defer ole.CoUninitialize()

	items, cleanup := wmiQueryItems(service,
		"SELECT SMBIOSBIOSVersion,SerialNumber FROM Win32_BIOS")
	defer cleanup()
	if len(items) == 0 {
		return BIOSInfo{}
	}
	it := items[0]
	return BIOSInfo{
		Version: strProp(it, "SMBIOSBIOSVersion"),
		Serial:  strProp(it, "SerialNumber"),
	}
}

// ---- Monitors ----

type MonitorInfo struct {
	Serial       string `json:"serial"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	SizeCM       int    `json:"size_cm"`
	Resolution   string `json:"resolution"`
}

func GetMonitors() []MonitorInfo {
	// We use WmiMonitorID from root\wmi namespace for model/serial
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// ok
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil
	}
	defer unknown.Release()
	wmiDisp, _ := unknown.QueryInterface(ole.IID_IDispatch)
	defer wmiDisp.Release()

	// Connect to root\wmi namespace
	serviceRaw, err := oleutil.CallMethod(wmiDisp, "ConnectServer", nil, "root\\wmi")
	if err != nil {
		return nil
	}
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	monitors := []MonitorInfo{}

	// Get monitor IDs
	items, cleanup := wmiQueryItems(service, "SELECT ManufacturerName,UserFriendlyName,SerialNumberID FROM WmiMonitorID")
	defer cleanup()

	// Get display size params
	paramItems, paramCleanup := wmiQueryItems(service,
		"SELECT MaxHorizontalImageSize,MaxVerticalImageSize FROM WmiMonitorBasicDisplayParams")
	defer paramCleanup()

	// Get resolution from standard WMI
	var resolution string
	{
		wmi2, service2, err2 := wmiConnect()
		if err2 == nil {
			defer wmi2.Release()
			defer service2.Release()
			resItems, resCleanup := wmiQueryItems(service2,
				"SELECT CurrentHorizontalResolution,CurrentVerticalResolution FROM Win32_VideoController")
			defer resCleanup()
			if len(resItems) > 0 {
				h := intProp(resItems[0], "CurrentHorizontalResolution")
				v := intProp(resItems[0], "CurrentVerticalResolution")
				if h > 0 && v > 0 {
					resolution = fmt.Sprintf("%dx%d", h, v)
				}
			}
		}
	}

	for i, it := range items {
		// Decode byte-array properties (WMI returns as []uint16 / []interface{})
		model := decodeWMIByteArray(it, "UserFriendlyName")
		if model == "" {
			continue
		}
		serial := decodeWMIByteArray(it, "SerialNumberID")
		manufacturer := decodeWMIByteArray(it, "ManufacturerName")
		if serial == "" {
			serial = "N/A"
		}

		sizeCM := 0
		if i < len(paramItems) {
			w := int(intProp(paramItems[i], "MaxHorizontalImageSize"))
			h := int(intProp(paramItems[i], "MaxVerticalImageSize"))
			if w > 0 && h > 0 {
				sizeCM = int(math.Round(math.Sqrt(float64(w*w + h*h))))
			}
		}

		monitors = append(monitors, MonitorInfo{
			Serial:       serial,
			Model:        model,
			Manufacturer: manufacturer,
			SizeCM:       sizeCM,
			Resolution:   resolution,
		})
	}
	return monitors
}

// decodeWMIByteArray reads a WMI byte-array property (e.g. UserFriendlyName) as a string.
// WMI returns these as a SAFEARRAY of uint16 values (the raw name chars), terminated by 0.
func decodeWMIByteArray(it *ole.IDispatch, propName string) string {
	v, err := oleutil.GetProperty(it, propName)
	if err != nil {
		return ""
	}
	// The value comes back as []interface{} where each element is uint16 or int
	raw, ok := v.Value().([]interface{})
	if !ok {
		return ""
	}
	b := make([]byte, 0, len(raw))
	for _, ch := range raw {
		var n uint16
		switch val := ch.(type) {
		case uint16:
			n = val
		case int16:
			n = uint16(val)
		default:
			continue
		}
		if n == 0 {
			break
		}
		b = append(b, byte(n))
	}
	return strings.TrimSpace(string(b))
}
