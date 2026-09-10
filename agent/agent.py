import threading
import time
import requests
import subprocess
import sys
import os
from core.config import load_config, save_config
from collectors.base import get_hw_uuid
from tasks.runner import collect_payload, sync_loop, poll_loop
from tasks.executor import execute_command
from core import logger

AGENT_VERSION = "3.0.0"

def parse_version(v_str):
    try:
        return tuple(map(int, (v_str.split("."))))
    except Exception:
        return (0, 0, 0)

def update_check_loop():
    logger.info("Updater", "Auto-update checker thread started...")
    # Give the agent a few seconds to run initial checks
    time.sleep(15)
    
    while True:
        try:
            config = load_config()
            headers = {"X-Agent-Token": config["agent_token"]}
            
            # Fetch latest version info
            url = f"{config['server_url']}/update/check"
            res = requests.get(url, headers=headers, timeout=15)
            if res.status_code == 200:
                update_info = res.json()
                latest_ver = update_info.get("version", "")
                download_path = update_info.get("download_url", "")
                
                # Check if version is newer
                if latest_ver and latest_ver != AGENT_VERSION:
                    if parse_version(latest_ver) > parse_version(AGENT_VERSION):
                        logger.info("Updater", f"New version {latest_ver} detected (current: {AGENT_VERSION}). Downloading...")
                        
                        # Download updated agent script
                        download_url = f"{config['server_url']}{download_path}" if download_path.startswith("/") else download_path
                        dl_res = requests.get(download_url, headers=headers, timeout=30)
                        if dl_res.status_code == 200:
                            with open("agent_new.py", "wb") as f:
                                f.write(dl_res.content)
                            
                            logger.info("Updater", "Download complete. Launching updater and exiting.")
                            # Start updater script in separate process
                            subprocess.Popen([sys.executable, "updater.py"])
                            os._exit(0) # Use os._exit(0) to immediately shut down all agent threads and unlock files
                        else:
                            logger.error("Updater", f"Failed to download update file: {dl_res.status_code}")
            else:
                logger.error("Updater", f"Check update returned status: {res.status_code}")
        except Exception as e:
            logger.error("Updater", f"Error in update check loop: {e}")
        
        # Check every 60 minutes
        time.sleep(3600)

if __name__ == "__main__":
    print(f"Yuna Agent v{AGENT_VERSION} Started (Modular)")
    print(f"UUID: {get_hw_uuid()}")
    
    # Start threads
    threading.Thread(target=sync_loop, daemon=True).start()
    threading.Thread(target=poll_loop, daemon=True).start()
    threading.Thread(target=update_check_loop, daemon=True).start()
    
    # Keep main alive
    while True:
        time.sleep(1)
