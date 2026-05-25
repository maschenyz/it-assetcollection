import json
import platform
import subprocess


def _run_powershell_json(ps_command: str):
    """
    Runs a PowerShell command that outputs JSON and returns parsed Python data.
    Returns None on failure.
    """
    if platform.system() != "Windows":
        return None
    try:
        cmd = [
            "powershell",
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-Command",
            ps_command,
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, shell=False, timeout=60)
        if not result.stdout:
            return None
        return json.loads(result.stdout)
    except Exception:
        return None


def get_hotfixes(limit: int = 200):
    """Returns a list of recent Windows hotfixes (KBs)."""
    data = _run_powershell_json(
        f"Get-HotFix | Sort-Object InstalledOn -Descending | "
        f"Select-Object -First {int(limit)} HotFixID,Description,InstalledOn,InstalledBy | "
        f"ConvertTo-Json -Depth 3"
    )
    if not data:
        return []
    if isinstance(data, dict):
        data = [data]
    out = []
    for item in data:
        if not isinstance(item, dict):
            continue
        kb = item.get("HotFixID") or ""
        if kb:
            out.append(
                {
                    "kb": str(kb),
                    "description": str(item.get("Description") or ""),
                    "installed_on": str(item.get("InstalledOn") or ""),
                    "installed_by": str(item.get("InstalledBy") or ""),
                }
            )
    return out


def get_defender_status():
    """Returns key Microsoft Defender status fields."""
    data = _run_powershell_json(
        "try { "
        "Get-MpComputerStatus | "
        "Select-Object AMServiceEnabled,AntivirusEnabled,AntispywareEnabled,RealTimeProtectionEnabled,"
        "NISEnabled,BehaviorMonitorEnabled,IoavProtectionEnabled,OnAccessProtectionEnabled,"
        "AntivirusSignatureVersion,AntivirusSignatureLastUpdated,EngineVersion,AMEngineVersion,"
        "QuickScanAge,FullScanAge | "
        "ConvertTo-Json -Depth 3 "
        "} catch { '{}' }"
    )
    if not data or not isinstance(data, dict):
        return {}
    # Keep it small and stable
    return {
        "am_service_enabled": bool(data.get("AMServiceEnabled")) if "AMServiceEnabled" in data else None,
        "antivirus_enabled": bool(data.get("AntivirusEnabled")) if "AntivirusEnabled" in data else None,
        "antispyware_enabled": bool(data.get("AntispywareEnabled")) if "AntispywareEnabled" in data else None,
        "realtime_protection_enabled": bool(data.get("RealTimeProtectionEnabled")) if "RealTimeProtectionEnabled" in data else None,
        "nis_enabled": bool(data.get("NISEnabled")) if "NISEnabled" in data else None,
        "engine_version": str(data.get("EngineVersion") or ""),
        "antivirus_signature_version": str(data.get("AntivirusSignatureVersion") or ""),
        "antivirus_signature_last_updated": str(data.get("AntivirusSignatureLastUpdated") or ""),
        "quick_scan_age_days": data.get("QuickScanAge"),
        "full_scan_age_days": data.get("FullScanAge"),
    }


def get_startup_items():
    """
    Returns startup entries from common Run keys (HKLM/HKCU).
    Output: list[{scope, name, command}]
    """
    raw = _run_powershell_json(
        r"$out = @();"
        r"$paths = @("
        r"'HKLM:\Software\Microsoft\Windows\CurrentVersion\Run',"
        r"'HKLM:\Software\Microsoft\Windows\CurrentVersion\RunOnce',"
        r"'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run',"
        r"'HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce'"
        r");"
        r"foreach ($p in $paths) {"
        r"  if (Test-Path $p) {"
        r"    $props = Get-ItemProperty -Path $p;"
        r"    foreach ($n in $props.PSObject.Properties.Name) {"
        r"      if ($n -like 'PS*') { continue }"
        r"      $val = $props.$n;"
        r"      if ($null -ne $val -and $val.ToString().Trim().Length -gt 0) {"
        r"        $scope = ($p.Substring(0,4));"
        r"        $out += [PSCustomObject]@{ scope=$scope; name=$n; command=$val.ToString() };"
        r"      }"
        r"    }"
        r"  }"
        r"}"
        r"$out | ConvertTo-Json -Depth 4"
    )
    if not raw:
        return []
    if isinstance(raw, dict):
        raw = [raw]
    out = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        name = str(item.get("name") or "")
        cmd = str(item.get("command") or "")
        if not name or not cmd:
            continue
        out.append({"scope": str(item.get("scope") or ""), "name": name, "command": cmd})
    return out


def get_services(limit: int = 400):
    """Returns services inventory (limited for payload size)."""
    data = _run_powershell_json(
        f"Get-CimInstance Win32_Service | Sort-Object Name | "
        f"Select-Object -First {int(limit)} Name,DisplayName,State,StartMode,PathName | "
        f"ConvertTo-Json -Depth 4"
    )
    if not data:
        return []
    if isinstance(data, dict):
        data = [data]
    out = []
    for item in data:
        if not isinstance(item, dict):
            continue
        out.append(
            {
                "name": str(item.get("Name") or ""),
                "display_name": str(item.get("DisplayName") or ""),
                "state": str(item.get("State") or ""),
                "start_mode": str(item.get("StartMode") or ""),
                "path": str(item.get("PathName") or ""),
            }
        )
    return out


def get_wifi_status():
    """Returns SSID/BSSID if connected via Wi-Fi, best-effort."""
    if platform.system() != "Windows":
        return {}
    try:
        cmd = ["netsh", "wlan", "show", "interfaces"]
        result = subprocess.run(cmd, capture_output=True, text=True, shell=False, timeout=20)
        text = result.stdout or ""
        ssid = ""
        bssid = ""
        state = ""
        signal = ""
        for line in text.splitlines():
            if ":" not in line:
                continue
            k, v = line.split(":", 1)
            k = k.strip().lower()
            v = v.strip()
            if k == "state":
                state = v
            elif k == "ssid":
                ssid = v
            elif k == "bssid":
                bssid = v
            elif k == "signal":
                signal = v
        if not ssid and not bssid and not state:
            return {}
        return {"state": state, "ssid": ssid, "bssid": bssid, "signal": signal}
    except Exception:
        return {}


def get_network_adapters(limit: int = 64):
    """Returns basic network adapter inventory via WMI (works on most hosts)."""
    data = _run_powershell_json(
        f"Get-CimInstance Win32_NetworkAdapter | "
        f"Where-Object {{$_.PhysicalAdapter -eq $true}} | "
        f"Sort-Object Name | "
        f"Select-Object -First {int(limit)} Name,MACAddress,NetEnabled,NetConnectionStatus,Speed,Manufacturer | "
        f"ConvertTo-Json -Depth 4"
    )
    if not data:
        return []
    if isinstance(data, dict):
        data = [data]
    out = []
    for item in data:
        if not isinstance(item, dict):
            continue
        out.append(
            {
                "name": str(item.get("Name") or ""),
                "mac": str(item.get("MACAddress") or ""),
                "enabled": bool(item.get("NetEnabled")) if "NetEnabled" in item else None,
                "status": item.get("NetConnectionStatus"),
                "speed": item.get("Speed"),
                "manufacturer": str(item.get("Manufacturer") or ""),
            }
        )
    return out


def get_smart_status(limit: int = 64):
    """
    Best-effort SMART failure prediction status.
    Output: list[{instance, predict_failure}]
    """
    data = _run_powershell_json(
        f"Get-WmiObject -Namespace root\\\\wmi -Class MSStorageDriver_FailurePredictStatus "
        f"| Select-Object -First {int(limit)} InstanceName,PredictFailure "
        f"| ConvertTo-Json -Depth 3"
    )
    if not data:
        return []
    if isinstance(data, dict):
        data = [data]
    out = []
    for item in data:
        if not isinstance(item, dict):
            continue
        out.append(
            {
                "instance": str(item.get("InstanceName") or ""),
                "predict_failure": bool(item.get("PredictFailure")) if "PredictFailure" in item else None,
            }
        )
    return out


def get_logged_on_users(limit: int = 16):
    """
    Best-effort logged-on users list.
    Uses `quser` output when available.
    """
    if platform.system() != "Windows":
        return []
    try:
        result = subprocess.run(["quser"], capture_output=True, text=True, shell=False, timeout=15)
        text = (result.stdout or "").strip()
        if not text:
            return []
        lines = [l.rstrip() for l in text.splitlines() if l.strip()]
        if len(lines) <= 1:
            return []
        # Skip header
        users = []
        for line in lines[1:]:
            # quser is column-based; username is the first token (may start with '>').
            parts = line.strip().split()
            if not parts:
                continue
            name = parts[0].lstrip(">")
            if name:
                users.append({"username": name})
            if len(users) >= limit:
                break
        return users
    except Exception:
        return []
