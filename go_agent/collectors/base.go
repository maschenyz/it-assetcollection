package collectors

import (
	"fmt"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/google/uuid"
	"go_agent/config"
	"golang.org/x/sys/windows/registry"
)

// GetHWUUID returns a stable persistent hardware UUID for this machine.
// Priority: config → SMBIOS UUID → Motherboard Serial → Registry MachineGuid → random UUIDv4
func GetHWUUID() string {
	cfg := config.Load()
	if cfg.DeviceUUID != "" {
		return cfg.DeviceUUID
	}

	// 1. Try SMBIOS UUID via WMI
	if id := getSMBIOSUUID(); id != "" {
		persist(cfg, id)
		return id
	}

	// 2. Try Motherboard Serial (generate UUIDv5 from it)
	if serial := getMotherboardSerial(); serial != "" {
		id := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("yuna.motherboard."+serial)).String()
		fmt.Printf("[UUID] UUIDv5 from motherboard serial (%s): %s\n", serial, id)
		persist(cfg, id)
		return id
	}

	// 3. Try Windows Registry MachineGuid
	if id := getRegistryMachineGUID(); id != "" {
		persist(cfg, id)
		return id
	}

	// 4. Fallback random UUIDv4
	id := uuid.New().String()
	fmt.Printf("[UUID] Generated random UUIDv4: %s\n", id)
	persist(cfg, id)
	return id
}

func persist(cfg *config.Config, id string) {
	cfg.DeviceUUID = id
	_ = config.Save(cfg)
	fmt.Printf("[UUID] Using UUID: %s\n", id)
}

func isValidUUID(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	invalid := []string{
		"00000000-0000-0000-0000-000000000000",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
		"default string", "none", "unknown", "",
	}
	for _, bad := range invalid {
		if v == bad {
			return false
		}
	}
	if strings.Contains(v, "to be filled") {
		return false
	}
	return true
}

func getSMBIOSUUID() string {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// already initialized is OK
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return ""
	}
	defer unknown.Release()

	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	defer wmi.Release()

	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer")
	if err != nil {
		return ""
	}
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	resultRaw, err := oleutil.CallMethod(service, "ExecQuery",
		"SELECT UUID FROM Win32_ComputerSystemProduct")
	if err != nil {
		return ""
	}
	result := resultRaw.ToIDispatch()
	defer result.Release()

	countVar, _ := oleutil.GetProperty(result, "Count")
	if countVar.Val == 0 {
		return ""
	}

	itemRaw, err := oleutil.CallMethod(result, "ItemIndex", 0)
	if err != nil {
		return ""
	}
	item := itemRaw.ToIDispatch()
	defer item.Release()

	uuidVar, err := oleutil.GetProperty(item, "UUID")
	if err != nil {
		return ""
	}
	val := fmt.Sprintf("%v", uuidVar.Value())
	if isValidUUID(val) {
		return strings.TrimSpace(val)
	}
	return ""
}

func getMotherboardSerial() string {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// already initialized
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return ""
	}
	defer unknown.Release()
	wmi, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return ""
	}
	defer wmi.Release()

	serviceRaw, err := oleutil.CallMethod(wmi, "ConnectServer")
	if err != nil {
		return ""
	}
	service := serviceRaw.ToIDispatch()
	defer service.Release()

	resultRaw, err := oleutil.CallMethod(service, "ExecQuery",
		"SELECT SerialNumber FROM Win32_BaseBoard")
	if err != nil {
		return ""
	}
	result := resultRaw.ToIDispatch()
	defer result.Release()

	countVar, _ := oleutil.GetProperty(result, "Count")
	if countVar.Val == 0 {
		return ""
	}
	itemRaw, err := oleutil.CallMethod(result, "ItemIndex", 0)
	if err != nil {
		return ""
	}
	item := itemRaw.ToIDispatch()
	defer item.Release()

	snVar, err := oleutil.GetProperty(item, "SerialNumber")
	if err != nil {
		return ""
	}
	val := strings.TrimSpace(fmt.Sprintf("%v", snVar.Value()))
	lower := strings.ToLower(val)
	bad := []string{"none", "unknown", "default string", "to be filled by o.e.m.", "to be filled", ""}
	for _, b := range bad {
		if lower == b {
			return ""
		}
	}
	if strings.HasPrefix(lower, "00000000") {
		return ""
	}
	return val
}

func getRegistryMachineGUID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}
