import platform
import wmi
import pythoncom
import win32evtlog
import xml.etree.ElementTree as ET
from core.config import load_config, save_config

def get_printer_metrics():
    """Scan and sum print logs since last check"""
    config = load_config()
    page_counts = config.get("printer_page_counts", {})
    last_id = config.get("last_processed_event_record_id", 0)
    
    log_path = "Microsoft-Windows-PrintService/Operational"
    query_str = f"*[System[EventID=307 and EventRecordID > {last_id}]]"
    
    max_record_id = last_id
    
    try:
        handle = win32evtlog.EvtQuery(log_path, win32evtlog.EvtQueryChannelPath, query_str)
        while True:
            events = win32evtlog.EvtNext(handle, 100)
            if not events:
                break
                
            for event in events:
                try:
                    xml_str = win32evtlog.EvtRender(event, win32evtlog.EvtRenderEventXml)
                    root = ET.fromstring(xml_str)
                    
                    ns_sys = {'ns': 'http://schemas.microsoft.com/win/2004/08/events/event'}
                    rec_elem = root.find(".//ns:EventRecordID", ns_sys)
                    if rec_elem is not None and rec_elem.text:
                        rec_id = int(rec_elem.text)
                        if rec_id > max_record_id:
                            max_record_id = rec_id
                            
                    ns_user = {'ns': 'http://manifests.microsoft.com/win/2005/08/windows/printing/spooler/core/events'}
                    p5_elem = root.find(".//ns:Param5", ns_user)
                    p8_elem = root.find(".//ns:Param8", ns_user)
                    
                    if p5_elem is not None and p8_elem is not None and p5_elem.text and p8_elem.text:
                        printer_name = p5_elem.text.strip()
                        pages = int(p8_elem.text)
                        
                        if not (printer_name.startswith('{') and printer_name.endswith('}')):
                            page_counts[printer_name] = page_counts.get(printer_name, 0) + pages
                except Exception as e:
                    pass
    except Exception as e:
        print(f"[PrinterLog] EvtQuery failed: {e}")
        
    config["printer_page_counts"] = page_counts
    config["last_processed_event_record_id"] = max_record_id
    save_config(config)
    return page_counts

def get_printers_info():
    """Scan connected printers using WMI and get print log metrics"""
    pythoncom.CoInitialize()
    printers_info = []
    if platform.system() != "Windows":
        return printers_info
        
    try:
        c = wmi.WMI()
        page_counts = get_printer_metrics()
        drivers_info = {}
        
        for driver in c.Win32_PrinterDriver():
            name_key = driver.Name.split(",")[0] if driver.Name else ""
            drivers_info[name_key] = {
                "driver_version": getattr(driver, "DriverVersion", "Unknown"),
                "manufacturer": getattr(driver, "Manufacturer", "Unknown")
            }

        seen_printers = set()
        for printer in c.Win32_Printer():
            name = getattr(printer, "Name", "Unknown")
            if name in seen_printers:
                continue
            seen_printers.add(name)
            
            driver_name = getattr(printer, "DriverName", "")
            driver_details = drivers_info.get(driver_name, {"driver_version": "Unknown", "manufacturer": "Unknown"})
            
            total_pages = page_counts.get(name, 0)
            
            printers_info.append({
                "name": name,
                "model": getattr(printer, "DeviceID", "Unknown"),
                "port": getattr(printer, "PortName", "N/A"),
                "network": getattr(printer, "Network", False),
                "manufacturer": driver_details["manufacturer"],
                "driver_version": driver_details["driver_version"],
                "total_pages_printed": total_pages
            })
    except Exception as e:
        print(f"[Printer] WMI scan error: {e}")
        
    return printers_info
