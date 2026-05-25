import platform
import os
import wmi
import pythoncom
import math

def get_system_info():
    """Detailed System Info"""
    info = {
        "hostname": platform.node(),
        "os": platform.system(),
        "release": platform.release(),
        "version": platform.version(),
        "architecture": platform.machine(),
        "windows_build": "",
        "caption": "",
        "bios_serial": "",
        "last_user": os.getlogin() if hasattr(os, 'getlogin') else ""
    }
    
    if platform.system() == "Windows":
        try:
            pythoncom.CoInitialize()
            c = wmi.WMI()
            # BIOS Serial
            for bios in c.Win32_BIOS():
                info["bios_serial"] = bios.SerialNumber
            # Windows Build and OS Caption
            for os_info in c.Win32_OperatingSystem():
                info["windows_build"] = os_info.BuildNumber
                info["caption"] = getattr(os_info, "Caption", "")
        except:
            pass
            
    return info

def collect_monitors():
    """Scan connected monitors using WMI (Win32_DesktopMonitor + WmiMonitorID)"""
    pythoncom.CoInitialize()
    monitors = []
    if platform.system() != "Windows":
        return monitors
    try:
        c = wmi.WMI(namespace="root\\wmi")
        for mon in c.WmiMonitorID():
            try:
                # Decode byte arrays to string
                def decode_bytes(arr):
                    if not arr:
                        return ""
                    return "".join(chr(b) for b in arr if b != 0).strip()

                serial      = decode_bytes(getattr(mon, "SerialNumberID", []))
                model       = decode_bytes(getattr(mon, "UserFriendlyName", []))
                manufacturer= decode_bytes(getattr(mon, "ManufacturerName", []))

                # Get physical size from WmiMonitorBasicDisplayParams
                size_cm = 0
                resolution = ""
                try:
                    c2 = wmi.WMI(namespace="root\\wmi")
                    params_list = c2.WmiMonitorBasicDisplayParams()
                    if params_list:
                        p = params_list[0]  # Match by index if multiple monitors
                        w_cm = getattr(p, "MaxHorizontalImageSize", 0)  # cm
                        h_cm = getattr(p, "MaxVerticalImageSize", 0)    # cm
                        if w_cm and h_cm:
                            diag_cm = math.sqrt(w_cm**2 + h_cm**2)
                            size_cm = round(diag_cm)
                except Exception:
                    pass

                # Get resolution from Win32_VideoController (current display mode)
                try:
                    c3 = wmi.WMI()
                    for vc in c3.Win32_VideoController():
                        h = getattr(vc, "CurrentHorizontalResolution", 0)
                        v = getattr(vc, "CurrentVerticalResolution", 0)
                        if h and v:
                            resolution = f"{h}x{v}"
                            break
                except Exception:
                    pass

                if model:  # Only add if we got a valid model name
                    monitors.append({
                        "serial":       serial or "N/A",
                        "model":        model,
                        "manufacturer": manufacturer,
                        "size_cm":      size_cm,
                        "resolution":   resolution
                    })
            except Exception as e:
                print(f"[Monitor] Error reading monitor entry: {e}")
    except Exception as e:
        print(f"[Monitor] WMI namespace error: {e}")
        # Fallback: use Win32_DesktopMonitor
        try:
            c = wmi.WMI()
            for mon in c.Win32_DesktopMonitor():
                name = getattr(mon, "Name", "") or ""
                if name and name.strip():
                    monitors.append({
                        "serial":       "N/A",
                        "model":        name.strip(),
                        "manufacturer": "",
                        "size_cm":      0,
                        "resolution":   ""
                    })
        except Exception as e2:
            print(f"[Monitor] Fallback error: {e2}")
    return monitors

def get_hardware_info():
    """Deep Hardware Scan using WMI"""
    pythoncom.CoInitialize()
    hw = {
        "cpu": {},
        "ram": [],
        "storage": [],
        "gpu": [],
        "motherboard": {},
        "bios": {},
        "printers": []
    }
    
    if platform.system() == "Windows":
        try:
            c = wmi.WMI()
            
            # CPU Detail
            for processor in c.Win32_Processor():
                hw["cpu"] = {
                    "name": processor.Name.strip(),
                    "cores": processor.NumberOfCores,
                    "threads": processor.NumberOfLogicalProcessors,
                    "max_speed": processor.MaxClockSpeed,
                    "l3_cache": getattr(processor, "L3CacheSize", 0)
                }
            
            # RAM Slots Detail
            for mem in c.Win32_PhysicalMemory():
                hw["ram"].append({
                    "capacity_gb": round(int(mem.Capacity) / (1024**3), 2),
                    "speed": mem.Speed,
                    "manufacturer": mem.Manufacturer.strip(),
                    "part_number": mem.PartNumber.strip(),
                    "serial": mem.SerialNumber.strip()
                })
            
            # Physical Disks (Not partitions)
            for disk in c.Win32_DiskDrive():
                hw["storage"].append({
                    "model": disk.Model,
                    "size_gb": round(int(disk.Size) / (1024**3), 2),
                    "serial": disk.SerialNumber.strip() if disk.SerialNumber else "N/A",
                    "interface": disk.InterfaceType,
                    "media_type": disk.MediaType
                })
            
            # GPU Detail
            for gpu in c.Win32_VideoController():
                try:
                    raw_ram = getattr(gpu, "AdapterRAM", 0)
                    ram_mb = round(int(raw_ram) / (1024**2), 2) if raw_ram else 0
                except (ValueError, TypeError):
                    ram_mb = 0
                hw["gpu"].append({
                    "name": getattr(gpu, "Name", "Unknown"),
                    "driver_version": getattr(gpu, "DriverVersion", "N/A"),
                    "ram_mb": ram_mb
                })
            
            # Motherboard
            for board in c.Win32_BaseBoard():
                hw["motherboard"] = {
                    "manufacturer": board.Manufacturer,
                    "product": board.Product,
                    "version": board.Version
                }

            # BIOS
            for b in c.Win32_BIOS():
                hw["bios"] = {
                    "version": b.SMBIOSBIOSVersion,
                    "serial": b.SerialNumber,
                }
            
            # Import and populate printers information
            try:
                from collectors.printers import get_printers_info
                hw["printers"] = get_printers_info()
            except Exception as pe:
                print(f"[HW] Printer Scan Error: {pe}")

        except Exception as e:
            print(f"[HW] WMI Error: {e}")
            
    return hw
