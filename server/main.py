from fastapi import FastAPI, HTTPException, Header, Depends, Body
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel
from typing import Optional, List, Dict, Any
import psycopg2
from psycopg2.extras import RealDictCursor, Json
from psycopg2.pool import SimpleConnectionPool
import datetime
import os
import json
import jwt
import bcrypt
import requests
from dotenv import load_dotenv
from apscheduler.schedulers.background import BackgroundScheduler
from contextlib import contextmanager

# Load environment variables
load_dotenv()

app = FastAPI(title="Yuna Asset Management API")

# CORS Settings
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# Configuration
DB_CONFIG = {
    "host": os.getenv("DB_HOST", "localhost"),
    "database": os.getenv("DB_NAME", "asset"),
    "user": os.getenv("DB_USER", "postgres"),
    "password": os.getenv("DB_PASSWORD", "12123"),
    "port": os.getenv("DB_PORT", "5432")
}
AGENT_TOKEN = os.getenv("AGENT_SECRET_TOKEN", "yuna_secret_token_2024")
JWT_SECRET = os.getenv("JWT_SECRET", "super_secret_jwt_key_2026")
JWT_ALGORITHM = os.getenv("JWT_ALGORITHM", "HS256")
LINE_TOKEN = os.getenv("LINE_NOTIFY_TOKEN")
TG_BOT_TOKEN = os.getenv("TELEGRAM_BOT_TOKEN")
TG_CHAT_ID = os.getenv("TELEGRAM_CHAT_ID")

# DB Connection Pool
try:
    db_pool = SimpleConnectionPool(1, 20, **DB_CONFIG)
except Exception as e:
    print(f"Critical Error: Could not connect to database: {e}")
    db_pool = None

@contextmanager
def get_db():
    conn = db_pool.getconn()
    conn.autocommit = True
    try:
        yield conn
    finally:
        db_pool.putconn(conn)

# --- Helpers ---

def send_telegram(message: str):
    if not TG_BOT_TOKEN or not TG_CHAT_ID:
        return
    url = f"https://api.telegram.org/bot{TG_BOT_TOKEN}/sendMessage"
    try:
        requests.post(url, json={"chat_id": TG_CHAT_ID, "text": message, "parse_mode": "HTML"})
    except Exception as e:
        print(f"Telegram Error: {e}")

def send_line(message: str):
    if not LINE_TOKEN:
        return
    url = "https://notify-api.line.me/api/notify"
    headers = {"Authorization": f"Bearer {LINE_TOKEN}"}
    try:
        requests.post(url, headers=headers, data={"message": message})
    except Exception as e:
        print(f"Line Error: {e}")

# --- Models ---

class AgentCheckin(BaseModel):
    uuid: str
    hostname: str
    device_type: str
    ip_address: str
    mac_address: str
    os_info: Dict[str, Any]
    windows_build: Optional[str] = ""
    bios_serial: Optional[str] = ""
    last_user: Optional[str] = ""
    hardware: Dict[str, Any]
    software_list: List[Dict[str, Any]]
    printers: Optional[List[Dict[str, Any]]] = []
    usb_devices: Optional[List[Dict[str, Any]]] = []
    stats: Dict[str, Any] # cpu_pct, ram_pct, disk_pct

class LoginRequest(BaseModel):
    username: str
    password: str

# --- Middleware/Auth ---

async def verify_agent(x_agent_token: str = Header(None)):
    if x_agent_token != AGENT_TOKEN:
        raise HTTPException(status_code=401, detail="Invalid Agent Token")

async def verify_admin(authorization: str = Header(None)):
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="Unauthorized")
    token = authorization.split(" ")[1]
    try:
        payload = jwt.decode(token, JWT_SECRET, algorithms=[JWT_ALGORITHM])
        return payload
    except Exception:
        raise HTTPException(status_code=401, detail="Invalid Session")

# --- Agent Endpoints ---

@app.post("/api/v1/agent/checkin")
async def agent_checkin(payload: AgentCheckin, x_agent_token: str = Header(None)):
    await verify_agent(x_agent_token)
    
    # Clean input strings from NUL characters
    hostname = payload.hostname.replace('\x00', '').strip()
    bios_serial = (payload.bios_serial or "").replace('\x00', '').strip()
    last_user = (payload.last_user or "").replace('\x00', '').strip()
    ip_address = payload.ip_address.replace('\x00', '').strip()
    
    with get_db() as conn:
        cur = conn.cursor()
        
        # 1. Update Device Info
        cur.execute("""
            INSERT INTO asset.devices (uuid, hostname, device_type, mac_address, ip_address, os_info, windows_build, bios_serial, last_user, status, last_seen)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, 'ONLINE', CURRENT_TIMESTAMP)
            ON CONFLICT (uuid) DO UPDATE SET
                hostname = EXCLUDED.hostname,
                ip_address = EXCLUDED.ip_address,
                os_info = EXCLUDED.os_info,
                windows_build = EXCLUDED.windows_build,
                bios_serial = EXCLUDED.bios_serial,
                last_user = EXCLUDED.last_user,
                status = 'ONLINE',
                last_seen = CURRENT_TIMESTAMP;
        """, (payload.uuid, hostname, payload.device_type, payload.mac_address, ip_address, 
              Json(payload.os_info), payload.windows_build, bios_serial, last_user))
        
        # 2. Hardware Inventory
        hw = payload.hardware
        cur.execute("""
            INSERT INTO asset.hardware_inventory (device_id, cpu_info, ram_info, storage_info, gpu_info, motherboard_info, updated_at)
            VALUES (%s, %s, %s, %s, %s, %s, CURRENT_TIMESTAMP)
            ON CONFLICT (device_id) DO UPDATE SET
                cpu_info = EXCLUDED.cpu_info,
                ram_info = EXCLUDED.ram_info,
                storage_info = EXCLUDED.storage_info,
                gpu_info = EXCLUDED.gpu_info,
                motherboard_info = EXCLUDED.motherboard_info,
                updated_at = CURRENT_TIMESTAMP;
        """, (payload.uuid, Json(hw.get('cpu', "")), Json(hw.get('ram', "")), Json(hw.get('storage', "")), Json(hw.get('gpu', "")), Json({"board": hw.get('motherboard', ""), "bios": hw.get('bios', ""), "tpm": hw.get('tpm', ""), "printers": hw.get('printers', "")})))
        
        # 3. Heartbeat Log
        cur.execute("""
            INSERT INTO asset.heartbeats (device_id, ip_address, cpu_pct, ram_pct, disk_pct)
            VALUES (%s, %s, %s, %s, %s)
        """, (payload.uuid, payload.ip_address, payload.stats.get('cpu_pct',0), payload.stats.get('ram_pct',0), payload.stats.get('disk_pct',0)))
        
        # 4. Software Update (Simplified for speed)
        # In a real app, you might want to compare diffs, but for now we refresh.
        for sw in payload.software_list:
            # Clean NUL characters (0x00) from software name and version
            sw_name = sw['name'].replace('\x00', '').strip()
            sw_version = sw.get('version', '').replace('\x00', '').strip()
            sw_path = sw.get('install_path', '').replace('\x00', '').strip()

            if not sw_name: continue

            cur.execute("INSERT INTO asset.software_catalog (software_name) VALUES (%s) ON CONFLICT DO NOTHING RETURNING id", (sw_name,))
            res = cur.fetchone()
            sw_id = res[0] if res else None
            if not sw_id:
                cur.execute("SELECT id FROM asset.software_catalog WHERE software_name = %s", (sw_name,))
                sw_id = cur.fetchone()[0]
            
            cur.execute("""
                INSERT INTO asset.software_installations (device_id, software_id, version, install_path)
                VALUES (%s, %s, %s, %s)
                ON CONFLICT (device_id, software_id) DO UPDATE SET
                    version = EXCLUDED.version,
                    install_path = EXCLUDED.install_path,
                    detected_at = CURRENT_TIMESTAMP
            """, (payload.uuid, sw_id, sw_version, sw_path))

        return {"status": "ok", "timestamp": datetime.datetime.now().isoformat()}

@app.get("/api/v1/agent/tasks")
async def get_agent_tasks(uuid: str, x_agent_token: str = Header(None)):
    await verify_agent(x_agent_token)
    
    with get_db() as conn:
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute("""
            SELECT t.id, t.custom_script, c.cmd_name, c.script_content 
            FROM asset.task_queue t
            LEFT JOIN asset.command_templates c ON t.cmd_template_id = c.id
            WHERE t.device_id = %s AND t.status = 'pending' AND t.expires_at > CURRENT_TIMESTAMP
            ORDER BY t.created_at ASC
        """, (uuid,))
        tasks = cur.fetchall()
        
        if tasks:
            # Mark as received
            task_ids = [t['id'] for t in tasks]
            cur.execute("UPDATE asset.task_queue SET status = 'received', received_at = CURRENT_TIMESTAMP WHERE id = ANY(%s)", (task_ids,))
            
        return tasks

@app.post("/api/v1/agent/tasks/{task_id}/result")
async def post_task_result(task_id: int, payload: Dict[str, Any] = Body(...), x_agent_token: str = Header(None)):
    await verify_agent(x_agent_token)
    
    with get_db() as conn:
        cur = conn.cursor()
        status = payload.get("status", "success")
        output = payload.get("output", "")
        
        cur.execute("""
            UPDATE asset.task_queue 
            SET status = %s, result_output = %s, completed_at = CURRENT_TIMESTAMP 
            WHERE id = %s
        """, (status, str(output), task_id))
        
    return {"status": "recorded"}

@app.post("/api/v1/agent/screenshot")
async def post_screenshot(uuid: str, task_id: int, image_data: str = Body(...), x_agent_token: str = Header(None)):
    await verify_agent(x_agent_token)
    
    with get_db() as conn:
        cur = conn.cursor()
        cur.execute("""
            INSERT INTO asset.screenshots (device_id, image_data, task_id)
            VALUES (%s, %s, %s)
        """, (uuid, image_data, task_id))
        
    return {"status": "saved"}

# --- Dashboard Endpoints ---

@app.post("/api/v1/auth/login")
async def login(req: LoginRequest):
    # For now, use .env or DB. Let's check DB first.
    with get_db() as conn:
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute("SELECT * FROM asset.users WHERE username = %s AND is_active = TRUE", (req.username,))
        user = cur.fetchone()
        
        if user and bcrypt.checkpw(req.password.encode(), user['password_hash'].encode()):
            token = jwt.encode({
                "sub": user['username'],
                "role": user['role'],
                "exp": datetime.datetime.utcnow() + datetime.timedelta(hours=12)
            }, JWT_SECRET, algorithm=JWT_ALGORITHM)
            return {"access_token": token, "token_type": "bearer", "role": user['role']}
        
        # Fallback to .env admin
        if req.username == os.getenv("ADMIN_USERNAME") and req.password == os.getenv("ADMIN_PASSWORD"):
             token = jwt.encode({
                "sub": req.username,
                "role": "admin",
                "exp": datetime.datetime.utcnow() + datetime.timedelta(hours=12)
            }, JWT_SECRET, algorithm=JWT_ALGORITHM)
             return {"access_token": token, "token_type": "bearer", "role": "admin"}
             
    raise HTTPException(status_code=401, detail="Invalid credentials")

@app.get("/api/v1/dashboard/summary")
async def get_summary(user: dict = Depends(verify_admin)):
    with get_db() as conn:
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute("SELECT count(*) as total, count(*) FILTER (WHERE status = 'ONLINE') as online FROM asset.devices")
        stats = cur.fetchone()
        
        cur.execute("SELECT count(*) as count FROM asset.software_installations")
        sw_count = cur.fetchone()
        
        return {
            "devices": stats,
            "software_total": sw_count['count'],
            "alerts": 0 # Placeholder
        }

@app.get("/api/v1/devices")
async def list_devices(status: Optional[str] = None, user: dict = Depends(verify_admin)):
    with get_db() as conn:
        cur = conn.cursor(cursor_factory=RealDictCursor)
        query = "SELECT * FROM asset.devices"
        params = []
        if status:
            query += " WHERE status = %s"
            params.append(status)
        query += " ORDER BY last_seen DESC"
        cur.execute(query, params)
        return cur.fetchall()

@app.post("/api/v1/tasks")
async def create_task(device_uuid: str, template_id: Optional[int] = None, custom_script: Optional[str] = None, user: dict = Depends(verify_admin)):
    with get_db() as conn:
        cur = conn.cursor()
        cur.execute("""
            INSERT INTO asset.task_queue (device_id, cmd_template_id, custom_script, created_by)
            VALUES (%s, %s, %s, %s) RETURNING id
        """, (device_uuid, template_id, custom_script, user['sub']))
        task_id = cur.fetchone()[0]
        return {"task_id": task_id, "status": "pending"}

# --- Background Jobs ---

def check_offline_devices():
    print("[Job] Checking for offline devices...")
    with get_db() as conn:
        cur = conn.cursor()
        cur.execute("""
            UPDATE asset.devices 
            SET status = 'OFFLINE' 
            WHERE status = 'ONLINE' AND last_seen < (CURRENT_TIMESTAMP - INTERVAL '10 minutes')
        """)

def check_overtime_devices():
    # Job at 20:00 to alert if devices still on
    now = datetime.datetime.now()
    if now.hour == 20:
        print("[Job] Checking for overtime devices...")
        with get_db() as conn:
            cur = conn.cursor(cursor_factory=RealDictCursor)
            cur.execute("SELECT hostname FROM asset.devices WHERE status = 'ONLINE' AND allow_overtime = FALSE")
            devices = cur.fetchall()
            if devices:
                msg = "🚨 <b>เครื่องยังไม่ปิด (20:00 น.):</b>\n"
                msg += "\n".join([f"• {d['hostname']}" for d in devices])
                send_telegram(msg)

scheduler = BackgroundScheduler()
scheduler.add_job(check_offline_devices, 'interval', minutes=5)
scheduler.add_job(check_overtime_devices, 'cron', hour=20)
scheduler.start()

# --- Static Files ---
# ใช้ abspath เพื่อความชัวร์ว่าชี้ไปที่โฟลเดอร์ dashboard ในโปรเจกต์ปัจจุบัน
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEB_DIR = os.path.join(BASE_DIR, "web")

print(f"[*] Serving Web UI from: {WEB_DIR}")

if os.path.exists(WEB_DIR):
    # Serve web UI assets at root paths like /style.css, /app.js
    app.mount("/", StaticFiles(directory=WEB_DIR, html=True), name="web")

@app.get("/")
async def read_index():
    path = os.path.join(WEB_DIR, "index.html")
    if os.path.exists(path):
        return FileResponse(path)
    return {"message": "Web UI not found"}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)
