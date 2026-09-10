import subprocess
import json
import requests
import base64
from io import BytesIO
from PIL import ImageGrab
from core.config import load_config
from collectors.base import get_hw_uuid
from collectors.processes import get_running_processes, kill_process_by_pid
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

# Commands handled internally (not passed to shell)
INTERNAL_COMMANDS = {"screenshot", "get_processes", "scan_network"}

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
    script_lower = script.lower()
    
    # Check for defense in depth mapping
    if script in SYMBOLIC_MAP:
        logger.info("Executor", f"Mapping symbolic command '{script}' to standard Windows script: '{SYMBOLIC_MAP[script]}'")
        script = SYMBOLIC_MAP[script]
    elif script_lower in SYMBOLIC_MAP:
        logger.info("Executor", f"Mapping symbolic command '{script_lower}' to standard Windows script: '{SYMBOLIC_MAP[script_lower]}'")
        script = SYMBOLIC_MAP[script_lower]

    logger.info("Executor", f"Executing task {task_id}: {script}")
    result = ""
    status = "success"
    
    try:
        if script_lower == "screenshot":
            result = take_screenshot(task_id)
            if "error" in result.lower():
                status = "failed"

        elif script_lower == "get_processes":
            # Collect running processes and foreground window
            logger.info("Executor", f"Collecting process list for task {task_id}...")
            proc_data = get_running_processes(top_n=20)
            result = json.dumps(proc_data, ensure_ascii=False)
            logger.info("Executor", f"Process list collected: {len(proc_data.get('processes', []))} processes")

        elif script_lower.startswith("kill_process:"):
            # Kill a process by PID — format: "kill_process:1234"
            try:
                pid = int(script.split(":", 1)[1].strip())
                success, msg = kill_process_by_pid(pid)
                result = msg
                status = "success" if success else "failed"
                logger.info("Executor", f"Kill PID {pid}: {msg}")
            except (ValueError, IndexError):
                result = f"Invalid kill_process command format: '{script}'"
                status = "failed"
                logger.error("Executor", result)

        elif script_lower == "scan_network":
            result = "Network scan logic not implemented yet"
            logger.info("Executor", "Network scan logic requested but not implemented yet")

        else:
            # Run Windows command line
            res = subprocess.run(script, shell=True, capture_output=True, text=True, timeout=60)
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
