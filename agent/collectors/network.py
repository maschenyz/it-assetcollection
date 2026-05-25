import psutil
import platform
import subprocess

def get_network_info():
    """Get active network interface IP, MAC address and connection type (LAN/Wi-Fi)"""
    addrs = psutil.net_if_addrs()
    ip = None
    mac = None
    active_name = None
    
    # Find the active interface with a valid IPv4 address (excluding loopback)
    for name, info in addrs.items():
        temp_ip = None
        temp_mac = None
        for addr in info:
            if addr.family == 2: # IPv4
                if not addr.address.startswith("127."):
                    temp_ip = addr.address
            elif addr.family == -1 or (hasattr(psutil, 'AF_LINK') and addr.family == psutil.AF_LINK): # MAC address
                temp_mac = addr.address
        if temp_ip:
            ip = temp_ip
            mac = temp_mac or ""
            active_name = name
            break
            
    if not ip:
        return {"ip": "127.0.0.1", "mac": "", "type": "LAN"}
        
    # Detect connection type (LAN vs Wi-Fi) on Windows using WMI
    connection_type = "LAN" # Default fallback
    if platform.system() == "Windows":
        try:
            import wmi
            import pythoncom
            pythoncom.CoInitialize()
            c = wmi.WMI()
            # Look at connected adapters
            for adapter in c.Win32_NetworkAdapter(NetConnectionStatus=2):
                name = getattr(adapter, "Name", "") or ""
                desc = getattr(adapter, "Description", "") or ""
                name_lower = (name + " " + desc).lower()
                if any(x in name_lower for x in ["wireless", "wi-fi", "wlan", "802.11"]):
                    connection_type = "Wi-Fi"
                    break
                elif any(x in name_lower for x in ["ethernet", "lan", "gigabit", "realtek", "intel"]):
                    connection_type = "LAN"
        except Exception as e:
            print(f"[Network] Connection type detection error: {e}")
            # Optional fallback using interface name
            if active_name:
                active_lower = active_name.lower()
                if any(x in active_lower for x in ["wi-fi", "wireless", "wlan"]):
                    connection_type = "Wi-Fi"
                    
    return {"ip": ip, "mac": mac, "type": connection_type}

def get_wifi_ssid():
    """Returns SSID if connected via Wi-Fi (Windows best-effort)."""
    if platform.system() != "Windows":
        return ""
    try:
        result = subprocess.run(["netsh", "wlan", "show", "interfaces"], capture_output=True, text=True, shell=False, timeout=20)
        text = result.stdout or ""
        for line in text.splitlines():
            if ":" not in line:
                continue
            k, v = line.split(":", 1)
            if k.strip().lower() == "ssid":
                return v.strip()
    except Exception:
        return ""
    return ""

def get_system_stats():
    """Get real-time CPU, RAM, and Disk utilization percentages"""
    # CPU percentage (blocking interval of 1 second for accuracy)
    cpu_pct = psutil.cpu_percent(interval=1)
    
    # RAM utilization percentage
    ram_pct = psutil.virtual_memory().percent
    
    # Real disk percentage on Windows (C:\) with fallback to root (/)
    disk_pct = 0.0
    try:
        # Check system type
        if platform.system() == "Windows":
            disk_pct = psutil.disk_usage('C:\\').percent
        else:
            disk_pct = psutil.disk_usage('/').percent
    except Exception as e:
        print(f"[Stats] Disk usage tracking error: {e}")
        # Secondary fallback
        try:
            disk_pct = psutil.disk_usage('/').percent
        except:
            disk_pct = 0.0
            
    return {
        "cpu_pct": cpu_pct,
        "ram_pct": ram_pct,
        "disk_pct": disk_pct
    }
