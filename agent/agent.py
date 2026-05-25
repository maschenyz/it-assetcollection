import threading
import time
import requests
from core.config import load_config, save_config
from collectors.base import get_hw_uuid
from tasks.runner import collect_payload, sync_loop, poll_loop
from tasks.executor import execute_command

AGENT_VERSION = "3.0.0"

if __name__ == "__main__":
    print(f"Yuna Agent v{AGENT_VERSION} Started (Modular)")
    print(f"UUID: {get_hw_uuid()}")
    
    # Start threads
    threading.Thread(target=sync_loop, daemon=True).start()
    threading.Thread(target=poll_loop, daemon=True).start()
    
    # Keep main alive
    while True:
        time.sleep(1)
