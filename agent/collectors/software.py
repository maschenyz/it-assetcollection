import platform
import subprocess
import json

def get_software_list():
    """Fast Registry-based Software Scan + Custom Folder Detections"""
    software = []
    if platform.system() == "Windows":
        # 1. Custom Detections (e.g., BMS HOSxP XE4)
        try:
            import glob
            import os
            users_dir = os.environ.get("SystemDrive", "C:") + "\\Users"
            search_pattern = os.path.join(users_dir, "*", "AppData", "Roaming", "BMS", "HOSxPXE4")
            found_paths = glob.glob(search_pattern)
            for path in found_paths:
                if os.path.isdir(path):
                    software.append({
                        "name": "HOSxP XE4",
                        "version": "N/A",
                        "publisher": "BMS (Bangkok Medical Software)",
                        "install_path": path,
                        "install_date": ""
                    })
        except Exception:
            pass

        # 2. Registry-based Software Scan
        try:
            cmd = [
                "powershell",
                "-NoProfile",
                "-ExecutionPolicy",
                "Bypass",
                "-Command",
                "Get-ItemProperty "
                "HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*, "
                "HKLM:\\Software\\WOW6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*, "
                "HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\* "
                "| Select-Object DisplayName,DisplayVersion,Publisher,InstallLocation,InstallDate "
                "| ConvertTo-Json -Depth 3",
            ]
            result = subprocess.run(cmd, capture_output=True, text=True, shell=False, timeout=60)
            if result.stdout:
                data = json.loads(result.stdout)
                if isinstance(data, dict): data = [data]
                seen = set()
                # Initialize seen with already detected software
                for sw in software:
                    seen.add((sw["name"].lower(), sw["version"].lower(), sw["publisher"].lower()))
                for item in data:
                    name = (item.get("DisplayName") or "").strip()
                    if not name:
                        continue
                    version = (item.get("DisplayVersion") or "").strip() or "N/A"
                    publisher = (item.get("Publisher") or "").strip()
                    install_path = (item.get("InstallLocation") or "").strip() or "N/A"
                    install_date = (item.get("InstallDate") or "").strip()

                    key = (name.lower(), version.lower(), publisher.lower())
                    if key in seen:
                        continue
                    seen.add(key)

                    software.append({
                        "name": name,
                        "version": version,
                        "publisher": publisher,
                        "install_path": install_path,
                        "install_date": install_date,
                    })
        except:
            pass
    return software
