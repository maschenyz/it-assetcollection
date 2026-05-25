import platform
import subprocess
import uuid
import wmi
import pythoncom
from core.config import load_config, save_config

_cached_uuid = None

def get_wmic_uuid():
    try:
        output = subprocess.check_output("wmic csproduct get uuid", shell=True, text=True)
        lines = [line.strip() for line in output.split('\n') if line.strip()]
        if len(lines) > 1:
            val = lines[1]
            val_clean = val.strip().lower()
            if val_clean not in [
                "00000000-0000-0000-0000-000000000000",
                "ffffffff-ffff-ffff-ffff-ffffffffffff",
                "default string",
                "none",
                "unknown"
            ] and "to be filled" not in val_clean:
                return val.strip()
    except:
        pass
    return None

def get_motherboard_serial():
    pythoncom.CoInitialize()
    try:
        c = wmi.WMI()
        for board in c.Win32_BaseBoard():
            val = board.SerialNumber
            if val:
                val_clean = val.strip().lower()
                if val_clean not in ["none", "unknown", "default string", "to be filled by o.e.m.", "to be filled"] and not val_clean.startswith("00000000"):
                    return val.strip()
    except:
        pass
    
    # Fallback through subprocess wmic
    try:
        output = subprocess.check_output("wmic baseboard get serialnumber", shell=True, text=True)
        lines = [line.strip() for line in output.split('\n') if line.strip()]
        if len(lines) > 1:
            val = lines[1]
            val_clean = val.strip().lower()
            if val_clean not in ["none", "unknown", "default string", "to be filled by o.e.m.", "to be filled"] and not val_clean.startswith("00000000"):
                return val.strip()
    except:
        pass
    return None

def get_registry_machine_guid():
    if platform.system() == "Windows":
        try:
            import winreg
            key = winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, r"SOFTWARE\Microsoft\Cryptography")
            val, _ = winreg.QueryValueEx(key, "MachineGuid")
            winreg.CloseKey(key)
            if val:
                return val.strip()
        except:
            pass
    return None

def get_hw_uuid():
    """Get Hardware UUID with Config persistence, SMBIOS/Motherboard/Registry fallback (HWmonitor Style)"""
    global _cached_uuid
    if _cached_uuid:
        return _cached_uuid

    # Try reading from config first
    config = load_config()
    if "device_uuid" in config and config["device_uuid"]:
        _cached_uuid = config["device_uuid"]
        return _cached_uuid

    hardware_uuid = None

    # Try WMI Hardware SMBIOS UUID
    pythoncom.CoInitialize()
    try:
        c = wmi.WMI()
        for item in c.Win32_ComputerSystemProduct():
            val = item.UUID
            if val:
                val_clean = val.strip().lower()
                # Check for dummy or invalid UUIDs
                if val_clean not in [
                    "00000000-0000-0000-0000-000000000000",
                    "ffffffff-ffff-ffff-ffff-ffffffffffff",
                    "default string",
                    "none",
                    "unknown"
                ] and "to be filled" not in val_clean:
                    hardware_uuid = val.strip()
                    break
    except Exception as e:
        print(f"[UUID] WMI UUID check failed: {e}")

    # Try subprocess WMIC UUID as fallback
    if not hardware_uuid:
        wmic_uuid = get_wmic_uuid()
        if wmic_uuid:
            hardware_uuid = wmic_uuid

    # Try Motherboard Serial Number and hash it (UUIDv5)
    if not hardware_uuid:
        mb_serial = get_motherboard_serial()
        if mb_serial:
            # Generate stable UUIDv5 based on Motherboard Serial Number
            hardware_uuid = str(uuid.uuid5(uuid.NAMESPACE_DNS, f"yuna.motherboard.{mb_serial}"))
            print(f"[UUID] Generated stable UUIDv5 from Motherboard Serial ({mb_serial}): {hardware_uuid}")

    # Try Windows MachineGuid from Registry
    if not hardware_uuid:
        reg_guid = get_registry_machine_guid()
        if reg_guid:
            hardware_uuid = reg_guid

    # Fallback to persistent random UUIDv4
    if not hardware_uuid:
        hardware_uuid = str(uuid.uuid4())
        print(f"[UUID] Generated new random UUIDv4: {hardware_uuid}")
    else:
        print(f"[UUID] Using hardware SMBIOS/Motherboard UUID: {hardware_uuid}")

    # Save back to config to persist it
    config["device_uuid"] = hardware_uuid
    save_config(config)
    
    _cached_uuid = hardware_uuid
    return _cached_uuid
