import subprocess
import requests
import base64
from io import BytesIO
from PIL import ImageGrab
from core.config import load_config
from collectors.base import get_hw_uuid
from core import logger

# Backup mapping for symbolic command names just in case the server fails to resolve them (Defense in depth)
SYMBOLIC_MAP = {
    "lock_screen": "rundll32.exe user32.dll,LockWorkStation",
    "restart_now": "shutdown /r /f /t 0",
    "shutdown_now": "shutdown /s /f /t 0",
    "shutdown_60": "shutdown /s /f /t 60",
    "cancel_shutdown": "shutdown /a",
    "restart_spooler": "net stop spooler && net start spooler",
    "clear_temp": 'del /q /f /s "%TEMP%\\*" & del /q /f /s "C:\\Windows\\Temp\\*"',
    "flush_dns": "ipconfig /flushdns",
    "check_disk": "chkdsk C: /scan"
}

def take_screenshot(task_id):
    config = load_config()
    headers = {"X-Agent-Token": config["agent_token"]}
    try:
        logger.info("Executor", f"Capturing screenshot for task {task_id}...")
        shot = ImageGrab.grab()
        shot.thumbnail((1280, 720))
        buf = BytesIO()
        shot.save(buf, format="JPEG", quality=70)
        img_b64 = base64.b64encode(buf.getvalue()).decode()
        
        url = f"{config['server_url']}/agent/screenshot?uuid={get_hw_uuid()}&task_id={task_id}"
        requests.post(url, data=img_b64, headers=headers)
        logger.info("Executor", f"Screenshot sent successfully for task {task_id}")
        return "Screenshot sent"
    except Exception as e:
        logger.error("Executor", f"Screenshot error: {e}")
        return f"Screenshot error: {e}"

def execute_command(task_id, script):
    config = load_config()
    headers = {"X-Agent-Token": config["agent_token"]}
    
    # Clean the script identifier/content
    script = script.strip() if script else ""
    
    # Check for defense in depth mapping
    if script in SYMBOLIC_MAP:
        logger.info("Executor", f"Mapping symbolic command '{script}' to standard Windows script: '{SYMBOLIC_MAP[script]}'")
        script = SYMBOLIC_MAP[script]

    logger.info("Executor", f"Executing task {task_id}: {script}")
    result = ""
    status = "success"
    
    try:
        if script == "screenshot":
            result = take_screenshot(task_id)
        elif script == "scan_network":
            result = "Network scan logic not implemented yet"
            logger.info("Executor", "Network scan logic requested but not implemented yet")
        else:
            # Run Windows command line
            res = subprocess.run(script, shell=True, capture_output=True, text=True)
            result = (res.stdout or "") + (res.stderr or "")
            if res.returncode != 0:
                status = "failed"
                logger.error("Executor", f"Task {task_id} failed with exit code {res.returncode}")
                
        # Send result back
        requests.post(f"{config['server_url']}/agent/tasks/{task_id}/result", 
                      json={"status": status, "output": result}, headers=headers)
        logger.info("Executor", f"Task {task_id} result reported as '{status}'")
                      
    except Exception as e:
        logger.error("Executor", f"Task {task_id} execution exception: {e}")
        try:
            requests.post(f"{config['server_url']}/agent/tasks/{task_id}/result", 
                          json={"status": "failed", "output": str(e)}, headers=headers)
        except Exception as re:
            logger.error("Executor", f"Failed to report exception for task {task_id}: {re}")
