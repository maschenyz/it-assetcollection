import time
import requests
import threading
from core.config import load_config
from core import logger
from collectors.base import get_hw_uuid
from collectors.hardware import get_system_info, get_hardware_info, collect_monitors
from collectors.software import get_software_list
from collectors.network import get_network_info, get_system_stats, get_wifi_ssid
from collectors.usb import get_usb_devices
from collectors.windows import (
    get_hotfixes,
    get_defender_status,
    get_services,
    get_startup_items,
    get_network_adapters,
    get_wifi_status,
    get_smart_status,
    get_logged_on_users,
)
from tasks.executor import execute_command

def collect_payload():
    """Gathers all telemetry details from collectors into a single checkin payload"""
    sys_info = get_system_info()
    hw_info = get_hardware_info()
    net_info = get_network_info()
    stats = get_system_stats()  # Real stats including psutil disk usage
    
    # Inject network connection type into sys_info (saved as JSONB os_info on server)
    sys_info["connection_type"] = net_info.get("type", "LAN")
    ssid = get_wifi_ssid()
    if ssid:
        sys_info["wifi_ssid"] = ssid
    
    return {
        "uuid": get_hw_uuid(),
        "hostname": sys_info["hostname"],
        "device_type": "Workstation",
        "ip_address": net_info["ip"],
        "mac_address": net_info["mac"],
        "os_info": sys_info,
        "windows_build": sys_info["windows_build"],
        "bios_serial": sys_info["bios_serial"],
        "last_user": sys_info["last_user"],
        "hardware": hw_info,
        "software_list": get_software_list(),
        "usb_devices": get_usb_devices(),
        "monitors": collect_monitors(),
        "stats": stats,
        "facts": {
            "hotfixes": get_hotfixes(),
            "defender": get_defender_status(),
            "services": get_services(),
            "startup_items": get_startup_items(),
            "network_adapters": get_network_adapters(),
            "wifi": get_wifi_status(),
            "smart": get_smart_status(),
            "logged_on_users": get_logged_on_users(),
        },
    }

def sync_loop():
    backoff = 2
    max_backoff = 300
    
    while True:
        config = load_config()
        headers = {"X-Agent-Token": config["agent_token"]}
        try:
            logger.info("Sync", "Sending inventory payload to server...")
            payload = collect_payload()
            
            response = requests.post(f"{config['server_url']}/agent/checkin", json=payload, headers=headers)
            if response.status_code == 200:
                logger.info("Sync", "Inventory synced successfully.")
                backoff = 2  # Reset backoff on success
                # Wait for standard interval
                time.sleep(config["sync_interval_minutes"] * 60)
            else:
                body = ""
                try:
                    body = (response.text or "").strip()
                except Exception:
                    body = ""
                if body:
                    if len(body) > 600:
                        body = body[:600] + "..."
                    logger.error("Sync", f"Server returned error code {response.status_code}: {body}")
                else:
                    logger.error("Sync", f"Server returned error code {response.status_code}. Retrying...")
                time.sleep(backoff)
                backoff = min(backoff * 2, max_backoff)
        except Exception as e:
            logger.error("Sync", f"Failed to connect to server: {e}. Retrying in {backoff} seconds...")
            time.sleep(backoff)
            backoff = min(backoff * 2, max_backoff)

def poll_loop():
    backoff = 2
    max_backoff = 60
    
    while True:
        config = load_config()
        headers = {"X-Agent-Token": config["agent_token"]}
        uuid = get_hw_uuid()
        try:
            response = requests.get(f"{config['server_url']}/agent/tasks?uuid={uuid}", headers=headers)
            if response.status_code == 200:
                backoff = 2  # Reset backoff on success
                tasks = response.json()
                if tasks:
                    logger.info("Poll", f"Received {len(tasks)} tasks from queue.")
                for t in tasks:
                    threading.Thread(target=execute_command, args=(t['id'], t['command_type']), daemon=True).start()
                
                time.sleep(config["poll_interval_seconds"])
            else:
                logger.error("Poll", f"Server returned error code {response.status_code}. Retrying...")
                time.sleep(backoff)
                backoff = min(backoff * 2, max_backoff)
        except Exception as e:
            logger.error("Poll", f"Poll connection error: {e}. Retrying in {backoff} seconds...")
            time.sleep(backoff)
            backoff = min(backoff * 2, max_backoff)
