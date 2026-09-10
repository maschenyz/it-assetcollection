package collectors

import (
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
	"strconv"

	"go_agent/config"
	"go_agent/logger"
)

type PrinterInfo struct {
	Name              string `json:"name"`
	Model             string `json:"model"`
	Port              string `json:"port"`
	IsNetwork         bool   `json:"network"`
	Manufacturer      string `json:"manufacturer"`
	DriverVersion     string `json:"driver_version"`
	TotalPagesPrinted int    `json:"total_pages_printed"`
}

// ---- Event Log XML parsing ----

type eventXML struct {
	System struct {
		EventRecordID int64 `xml:"EventRecordID"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"UserData>PrintJobDetailedInfo,omitempty"`
}

// getPrintEventsXML uses wevtutil to query print job events (ID 307)
func getPrintEventsXML(lastRecordID int64) ([]eventXML, int64) {
	// wevtutil qe "Microsoft-Windows-PrintService/Operational" /q:"*[System[EventID=307]]" /f:xml /rd:false /e:root
	// We filter by record ID on our side (wevtutil doesn't easily support EventRecordID filter)
	args := []string{
		"qe",
		"Microsoft-Windows-PrintService/Operational",
		`/q:*[System[EventID=307]]`,
		"/f:xml",
		"/rd:false",
		"/e:root",
	}
	out, err := exec.Command("wevtutil", args...).Output()
	if err != nil {
		logger.Error("Printers", fmt.Sprintf("wevtutil failed: %v", err))
		return nil, lastRecordID
	}

	// Wrap in root element for xml parsing
	wrapped := "<wrapper>" + string(out) + "</wrapper>"
	type wrapper struct {
		Events []eventXML `xml:"Event"`
	}
	var w wrapper
	if err := xml.Unmarshal([]byte(wrapped), &w); err != nil {
		logger.Error("Printers", fmt.Sprintf("XML parse error: %v", err))
		return nil, lastRecordID
	}

	maxID := lastRecordID
	var filtered []eventXML
	for _, ev := range w.Events {
		if ev.System.EventRecordID > lastRecordID {
			filtered = append(filtered, ev)
			if ev.System.EventRecordID > maxID {
				maxID = ev.System.EventRecordID
			}
		}
	}
	return filtered, maxID
}

// getPrinterMetrics reads print events and accumulates page counts
func getPrinterMetrics() map[string]int {
	cfg := config.Load()
	pageCounts := cfg.PrinterPageCounts
	if pageCounts == nil {
		pageCounts = map[string]int{}
	}

	events, maxID := getPrintEventsXML(cfg.LastProcessedEventRecordID)
	for _, ev := range events {
		// Map EventData fields: Param5 = printer name, Param8 = pages
		dataMap := map[string]string{}
		for _, d := range ev.EventData.Data {
			dataMap[d.Name] = d.Value
		}
		printerName := strings.TrimSpace(dataMap["Param5"])
		pagesStr := strings.TrimSpace(dataMap["Param8"])
		if printerName == "" || strings.HasPrefix(printerName, "{") {
			continue
		}
		pages, err := strconv.Atoi(pagesStr)
		if err != nil || pages <= 0 {
			continue
		}
		pageCounts[printerName] += pages
		logger.Info("Printers", fmt.Sprintf("EventLog: +%d pages → %s", pages, printerName))
	}

	cfg.PrinterPageCounts = pageCounts
	cfg.LastProcessedEventRecordID = maxID
	_ = config.Save(cfg)
	return pageCounts
}

// GetPrintersInfo scans installed printers via WMI and combines with event log metrics
func GetPrintersInfo() []PrinterInfo {
	pageCounts := getPrinterMetrics()

	wmi, service, err := wmiConnect()
	if err != nil {
		logger.Error("Printers", fmt.Sprintf("WMI connect error: %v", err))
		return nil
	}
	defer wmi.Release()
	defer service.Release()
	defer oleUninitialize()

	// Collect driver info
	driverMap := map[string]struct {
		version      string
		manufacturer string
	}{}
	dItems, dCleanup := wmiQueryItems(service,
		"SELECT Name,DriverVersion,Manufacturer FROM Win32_PrinterDriver")
	defer dCleanup()
	for _, it := range dItems {
		name := strProp(it, "Name")
		// Driver names look like "DriverName,3,Windows x64" – take first part
		parts := strings.SplitN(name, ",", 2)
		key := strings.TrimSpace(parts[0])
		driverMap[key] = struct {
			version      string
			manufacturer string
		}{
			version:      strProp(it, "DriverVersion"),
			manufacturer: strProp(it, "Manufacturer"),
		}
	}

	// Collect printers
	pItems, pCleanup := wmiQueryItems(service,
		"SELECT Name,DeviceID,PortName,Network,DriverName FROM Win32_Printer")
	defer pCleanup()

	seen := map[string]bool{}
	result := []PrinterInfo{}
	for _, it := range pItems {
		name := strProp(it, "Name")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		driverName := strProp(it, "DriverName")
		di := driverMap[driverName]

		networkStr := strProp(it, "Network")
		isNetwork := strings.EqualFold(networkStr, "true") || networkStr == "1"

		result = append(result, PrinterInfo{
			Name:              name,
			Model:             strProp(it, "DeviceID"),
			Port:              strProp(it, "PortName"),
			IsNetwork:         isNetwork,
			Manufacturer:      di.manufacturer,
			DriverVersion:     di.version,
			TotalPagesPrinted: pageCounts[name],
		})
	}
	return result
}

// oleUninitialize is a convenience passthrough so we can defer it
func oleUninitialize() {
	// go-ole auto-tracks per-goroutine init/uninit via CoInitializeEx
	// We call our shared connector's uninit here
	// (no-op here because wmiConnect does it inline in this package)
}
