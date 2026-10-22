import platform
import os
import wmi
import pythoncom
import math
import psutil
import json
import subprocess

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
        c_wmi = wmi.WMI(namespace="root\\wmi")
        mon_ids = c_wmi.WmiMonitorID()
        params_list = c_wmi.WmiMonitorBasicDisplayParams()

        # ดึง Resolution รวมจาก Win32_VideoController
        resolution = ""
        try:
            c_win32 = wmi.WMI()
            for vc in c_win32.Win32_VideoController():
                h = getattr(vc, "CurrentHorizontalResolution", 0)
                v = getattr(vc, "CurrentVerticalResolution", 0)
                if h and v:
                    resolution = f"{h}x{v}"
                    break
        except Exception:
            pass

        def decode_bytes(arr):
            if not arr:
                return ""
            return "".join(chr(b) for b in arr if b != 0).strip()

        for idx, mon in enumerate(mon_ids):
            try:
                serial = decode_bytes(getattr(mon, "SerialNumberID", []))
                model = decode_bytes(getattr(mon, "UserFriendlyName", []))
                manufacturer = decode_bytes(
                    getattr(mon, "ManufacturerName", [])
                )

                size_inch = 0
                size_cm = 0

                # จับคู่ params ตาม Index เดียวกันกับ mon_ids
                if idx < len(params_list):
                    p = params_list[idx]
                    w_cm = getattr(p, "MaxHorizontalImageSize", 0)  # cm
                    h_cm = getattr(p, "MaxVerticalImageSize", 0)  # cm

                    if w_cm > 0 and h_cm > 0:
                        diag_cm = math.sqrt(w_cm**2 + h_cm**2)
                        size_cm = round(diag_cm)
                        size_inch = round(
                            diag_cm / 2.54
                        )  # แปลงเซนติเมตรเป็นนิ้ว

                if model:  # เพิ่มเฉพาะรายการที่มีชื่อรุ่น
                    monitors.append(
                        {
                            "serial": serial or "N/A",
                            "model": model,
                            "manufacturer": manufacturer,
                            "size_inch": size_inch,  # นิ้ว (เช่น 24, 27)
                            "size_cm": size_cm,  # เซนติเมตร
                            "resolution": resolution,
                        }
                    )
            except Exception as e:
                print(f"[Monitor] Error reading monitor entry: {e}")

    except Exception as e:
        print(f"[Monitor] WMI namespace error: {e}")
        # Fallback: ใช้ Win32_DesktopMonitor
        try:
            c = wmi.WMI()
            for mon in c.Win32_DesktopMonitor():
                name = getattr(mon, "Name", "") or ""
                if name and name.strip():
                    monitors.append(
                        {
                            "serial": "N/A",
                            "model": name.strip(),
                            "manufacturer": "",
                            "size_inch": 0,
                            "size_cm": 0,
                            "resolution": "",
                        }
                    )
        except Exception as e2:
            print(f"[Monitor] Fallback error: {e2}")

    return monitors


def collect_storage_volumes():
    """Collect logical disk capacity and free space for fixed local volumes."""
    if platform.system() != "Windows":
        return []

    volumes = []
    seen_drives = set()

    # =========================================================
    # 1. PSUTIL - Primary source
    # =========================================================
    try:
        for part in psutil.disk_partitions(all=False):

            drive = (part.mountpoint or "").strip()

            if not drive:
                continue

            if "cdrom" in (part.opts or "").lower():
                continue

            try:
                usage = psutil.disk_usage(drive)
            except (OSError, PermissionError):
                continue

            total = usage.total / (1024 ** 3)
            free = usage.free / (1024 ** 3)
            used = usage.used / (1024 ** 3)

            drive_key = drive.upper()

            if drive_key in seen_drives:
                continue

            volumes.append({
                "drive": drive,
                "label": "",
                "size_gb": round(total, 2),
                "used_gb": round(used, 2),
                "free_gb": round(free, 2),
                "usage_percent": round(usage.percent, 1),
            })

            seen_drives.add(drive_key)

    except Exception as exc:
        print(f"[Storage] PSUTIL error: {exc}")

    # =========================================================
    # 2. WMI - Supplement missing volumes
    # =========================================================
    try:
        pythoncom.CoInitialize()

        c = wmi.WMI()

        for disk in c.Win32_LogicalDisk():

            # DriveType 3 = Local Disk
            if int(getattr(disk, "DriveType", 0) or 0) != 3:
                continue

            drive = str(
                getattr(disk, "DeviceID", "") or ""
            ).strip()

            if not drive:
                continue

            drive_key = drive.upper()

            # PSUTIL already reported this drive
            if drive_key in seen_drives:
                continue

            try:
                total = float(disk.Size or 0) / (1024 ** 3)
                free = float(disk.FreeSpace or 0) / (1024 ** 3)
            except (ValueError, TypeError):
                continue

            used = max(0.0, total - free)

            volumes.append({
                "drive": drive,
                "label": str(
                    getattr(disk, "VolumeName", "") or ""
                ),
                "size_gb": round(total, 2),
                "used_gb": round(used, 2),
                "free_gb": round(free, 2),
                "usage_percent": round(
                    (used / total) * 100, 1
                ) if total else 0,
            })

            seen_drives.add(drive_key)

    except Exception as exc:
        print(f"[Storage] WMI LogicalDisk error: {exc}")

    # 3. PowerShell/CIM fallback for hosts where both Python providers are restricted.
    if not volumes:
        try:
            command = 'Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | Select-Object DeviceID,VolumeName,Size,FreeSpace | ConvertTo-Json -Depth 2'
            result = subprocess.run(
                ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command],
                capture_output=True, text=True, timeout=30, shell=False,
            )
            raw = json.loads(result.stdout) if result.stdout.strip() else []
            if isinstance(raw, dict):
                raw = [raw]
            for disk in raw:
                total = float(disk.get("Size") or 0) / (1024 ** 3)
                free = float(disk.get("FreeSpace") or 0) / (1024 ** 3)
                used = max(0.0, total - free)
                volumes.append({
                    "drive": str(disk.get("DeviceID") or ""),
                    "label": str(disk.get("VolumeName") or ""),
                    "size_gb": round(total, 2), "used_gb": round(used, 2),
                    "free_gb": round(free, 2),
                    "usage_percent": round((used / total) * 100, 1) if total else 0,
                })
        except Exception as exc:
            print(f"[Storage] PowerShell fallback error: {exc}")

    # =========================================================
    # 4. Sort drives A-Z
    # =========================================================
    volumes.sort(
        key=lambda x: x.get("drive", "")
    )

    return volumes


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
            
            # Physical Disks with S.M.A.R.T. Predict Failure
            # Step 1: Build a mapping of InstanceName -> predict_failure from MSStorageDriver_FailurePredictStatus
            smart_failure_map = {}
            try:
                c_wmi = wmi.WMI(namespace="root\\wmi")
                for smart in c_wmi.MSStorageDriver_FailurePredictStatus():
                    inst = getattr(smart, "InstanceName", "") or ""
                    # InstanceName format: "SCSI\\DISK&...\\X&Y&Z{N}" — split by __ to get device ID prefix
                    predict = getattr(smart, "PredictFailure", False) or False
                    # Use the first segment before "__" as key for partial matching
                    key = inst.split("\\")[-1].upper() if inst else ""
                    smart_failure_map[key] = bool(predict)
            except Exception:
                pass  # WMI SMART query not supported on this machine

            for disk in c.Win32_DiskDrive():
                # Map this disk's PNPDeviceID or DeviceID against SMART map
                pnp = (disk.PNPDeviceID or "").upper()
                device_id_short = pnp.split("\\")[-1].upper() if pnp else ""

                # Try to find a matching entry in the SMART failure map
                predict_failure = False
                for key, val in smart_failure_map.items():
                    if key and (key in pnp or device_id_short in key):
                        predict_failure = val
                        break
                else:
                    # Fallback: check Win32_DiskDrive.Status field
                    status = (getattr(disk, "Status", "") or "").upper()
                    if "PRED FAIL" in status:
                        predict_failure = True

                hw["storage"].append({
                    "model": disk.Model,
                    "size_gb": round(int(disk.Size) / (1024**3), 2) if disk.Size else 0,
                    "serial": disk.SerialNumber.strip() if disk.SerialNumber else "N/A",
                    "interface": disk.InterfaceType,
                    "media_type": disk.MediaType,
                    "predict_failure": predict_failure
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
