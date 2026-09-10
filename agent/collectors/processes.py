"""
Yuna Agent — Process Collector
Collects running processes, foreground window title, and resource usage.
Requires: psutil (already in requirements.txt)
Optional: pywin32 for richer window title detection
"""

import psutil
import platform
import logging
import time

logger = logging.getLogger("Yuna.Processes")

# Processes to skip (system noise)
SKIP_NAMES = {
    "system idle process", "system", "registry", "smss.exe", "csrss.exe",
    "wininit.exe", "services.exe", "lsass.exe", "svchost.exe", "conhost.exe",
    "runtimebroker.exe", "securityhealthservice.exe", "audiodg.exe",
    "dllhost.exe", "fontdrvhost.exe", "dwm.exe", "wuauclt.exe",
    "spoolsv.exe", "searchindexer.exe", "winlogon.exe", "userinit.exe",
}


def get_foreground_window() -> str:
    """Get the title of the currently active (foreground) window on Windows."""
    if platform.system() != "Windows":
        return ""
    try:
        import ctypes
        hwnd = ctypes.windll.user32.GetForegroundWindow()
        if not hwnd:
            return ""
        length = ctypes.windll.user32.GetWindowTextLengthW(hwnd)
        if length == 0:
            return ""
        buf = ctypes.create_unicode_buffer(length + 1)
        ctypes.windll.user32.GetWindowTextW(hwnd, buf, length + 1)
        return buf.value.strip()
    except Exception as e:
        logger.debug(f"[Processes] Could not get foreground window: {e}")
        return ""


def get_window_title_for_pid(pid: int) -> str:
    """Try to get the window title for a specific PID using win32gui if available."""
    try:
        import win32gui
        import win32process

        titles = []

        def callback(hwnd, _):
            if not win32gui.IsWindowVisible(hwnd):
                return
            _, found_pid = win32process.GetWindowThreadProcessId(hwnd)
            if found_pid == pid:
                title = win32gui.GetWindowText(hwnd).strip()
                if title:
                    titles.append(title)

        win32gui.EnumWindows(callback, None)
        return titles[0] if titles else ""
    except ImportError:
        # pywin32 not installed — skip silently
        return ""
    except Exception:
        return ""


def get_running_processes(top_n: int = 20) -> dict:
    """
    Collect top running processes sorted by CPU usage.

    Returns:
        {
          "foreground_window": "Counter-Strike 2",
          "processes": [
            {"pid": 1234, "name": "cs2.exe", "cpu_pct": 78.5, "ram_mb": 3200.0, "window_title": "Counter-Strike 2"},
            ...
          ]
        }
    """
    foreground = get_foreground_window()

    procs = []
    try:
        # First pass — initialize cpu_percent counters (will return 0.0 first call)
        alive = []
        for p in psutil.process_iter(['pid', 'name', 'cpu_percent', 'memory_info', 'status']):
            try:
                name = (p.info.get('name') or '').strip()
                if name.lower() in SKIP_NAMES:
                    continue
                if p.info.get('status') == psutil.STATUS_ZOMBIE:
                    continue
                # Prime the counter
                p.cpu_percent(interval=None)
                alive.append(p)
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                continue

        # Brief sleep so next cpu_percent() call has a delta to measure
        time.sleep(0.5)

        # Second pass — read actual CPU%
        for p in alive:
            try:
                name = (p.info.get('name') or '').strip()
                cpu = p.cpu_percent(interval=None)
                mem = p.memory_info()
                ram_mb = round(mem.rss / (1024 * 1024), 1) if mem else 0.0
                pid = p.pid

                procs.append({
                    "pid": pid,
                    "name": name,
                    "cpu_pct": round(cpu, 1),
                    "ram_mb": ram_mb,
                    "window_title": "",
                })
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                continue

    except Exception as e:
        logger.error(f"[Processes] Error collecting process list: {e}")
        return {"foreground_window": foreground, "processes": []}

    # Sort by CPU desc, fallback by RAM desc, take top N
    procs.sort(key=lambda x: (x["cpu_pct"], x["ram_mb"]), reverse=True)
    top = procs[:top_n]

    # Try to enrich window titles for top processes (best-effort, pywin32 optional)
    for proc in top:
        if proc["cpu_pct"] > 0.5 or proc["ram_mb"] > 50:
            try:
                title = get_window_title_for_pid(proc["pid"])
                if title:
                    proc["window_title"] = title
            except Exception:
                pass

    return {
        "foreground_window": foreground,
        "processes": top,
    }


def kill_process_by_pid(pid: int) -> tuple[bool, str]:
    """
    Attempt to kill a process by PID.
    Returns (success: bool, message: str)
    """
    try:
        p = psutil.Process(pid)
        name = p.name()
        p.terminate()  # SIGTERM first (graceful)
        try:
            p.wait(timeout=3)
        except psutil.TimeoutExpired:
            p.kill()  # SIGKILL if still alive
        return True, f"Process '{name}' (PID {pid}) terminated successfully"
    except psutil.NoSuchProcess:
        return False, f"Process PID {pid} not found"
    except psutil.AccessDenied:
        return False, f"Access denied: cannot terminate PID {pid} (system process?)"
    except Exception as e:
        return False, f"Error terminating PID {pid}: {e}"
