# Yuna Asset Management

ระบบจัดการสินทรัพย์และติดตามคอมพิวเตอร์ภายในองค์กร ประกอบด้วยเว็บแดชบอร์ด, REST API และ Windows agent สำหรับส่งข้อมูลเครื่องมายังศูนย์กลาง

## ความสามารถ

- แสดงภาพรวมอุปกรณ์และสถานะออนไลน์/ออฟไลน์
- เก็บข้อมูลฮาร์ดแวร์, ซอฟต์แวร์, เครือข่าย, USB, เครื่องพิมพ์ และจอภาพ
- จัดการรายการอุปกรณ์และส่งคำสั่งงานไปยัง agent
- เข้าสู่ระบบด้วย JWT
- ตั้งแจ้งเตือนผ่าน Telegram หรือ LINE Notify ได้
- Agent รองรับการตรวจหาเวอร์ชันใหม่และอัปเดตตัวเอง

## โครงสร้างโปรเจกต์

| โฟลเดอร์ | หน้าที่ |
| --- | --- |
| `server/` | FastAPI backend, API และ schema ของ PostgreSQL |
| `web/` | หน้าเว็บแดชบอร์ดแบบ static ที่ backend ให้บริการ |
| `agent/` | Python agent สำหรับ Windows (เวอร์ชันหลัก) |
| `go_agent/` | Go agent ตัวอย่าง/ทางเลือก |
| `dashboard-v1/` | แดชบอร์ดเวอร์ชันเดิม |

## สิ่งที่ต้องมี

- Python 3.10 ขึ้นไป
- PostgreSQL
- Windows สำหรับติดตั้ง Python agent
- (ทางเลือก) Go 1.25 ขึ้นไป สำหรับพัฒนา Go agent

## เริ่มต้นใช้งาน

### 1. สร้างฐานข้อมูล

สร้างฐานข้อมูลชื่อ `asset` ใน PostgreSQL แล้วรัน schema:

```powershell
psql -U postgres -d asset -f .\server\schema.sql
```

> ไฟล์ schema จะลบและสร้าง schema `asset` ใหม่ จึงไม่ควรรันบนฐานข้อมูลที่มีข้อมูลสำคัญโดยไม่สำรองข้อมูลก่อน

### 2. ตั้งค่า environment ของ backend

สร้างหรือแก้ไข `server/.env` โดยใช้ตัวอย่างนี้ และเก็บไฟล์นี้ไว้นอก Git:

```env
DB_HOST=localhost
DB_NAME=asset
DB_USER=postgres
DB_PASSWORD=change-me
DB_PORT=5432
AGENT_SECRET_TOKEN=replace-with-a-long-random-token
JWT_SECRET=replace-with-a-long-random-secret
JWT_ALGORITHM=HS256
LINE_NOTIFY_TOKEN=
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
```

### 3. ติดตั้งและรัน backend

```powershell
cd server
python -m venv venv
.\venv\Scripts\Activate.ps1
pip install -r requirements.txt
python main.py
```

เปิดแดชบอร์ดที่ [http://localhost:8080](http://localhost:8080) และเอกสาร API ที่ [http://localhost:8080/docs](http://localhost:8080/docs)

### 4. ติดตั้ง Python agent บนเครื่องลูกข่าย

แก้ไข `agent/agent_config.json` ให้ชี้ไปยังเซิร์ฟเวอร์จริงและใช้ token เดียวกับ `AGENT_SECRET_TOKEN`:

```json
{
  "server_url": "http://<server-host>:8080/api/v1",
  "agent_token": "<agent-secret-token>",
  "sync_interval_minutes": 30,
  "poll_interval_seconds": 30
}
```

จากนั้นรัน:

```powershell
cd agent
python -m venv venv
.\venv\Scripts\Activate.ps1
pip install -r requirements.txt
python agent.py
```

Agent จะส่ง inventory ไปยัง `POST /api/v1/agent/checkin` และตรวจสอบ task จากเซิร์ฟเวอร์ตามช่วงเวลาที่ตั้งค่าไว้

## API หลัก

| Endpoint | รายละเอียด |
| --- | --- |
| `POST /api/v1/auth/login` | เข้าสู่ระบบ |
| `GET /api/v1/dashboard/summary` | ข้อมูลสรุปของแดชบอร์ด |
| `GET /api/v1/devices` | รายการอุปกรณ์ |
| `POST /api/v1/agent/checkin` | รับข้อมูล inventory จาก agent |
| `GET /api/v1/agent/tasks` | ให้ agent ดึงงานที่รออยู่ |
| `POST /api/v1/tasks` | สร้างงานสำหรับ agent |

ดูรายการและรูปแบบ request/response ทั้งหมดได้จาก `/docs` ขณะ backend ทำงาน

## ข้อควรระวังด้านความปลอดภัย

- อย่า commit `server/.env`, `agent/agent_config.json`, log, executable หรือ virtual environment ขึ้น Git เพราะอาจมีข้อมูลลับหรือข้อมูลเฉพาะเครื่อง
- เปลี่ยนรหัสผ่านฐานข้อมูล, agent token และ JWT secret ก่อนใช้งานจริง
- ควรจำกัด CORS, ใช้ HTTPS และตั้ง firewall ให้ agent เข้าถึงได้เฉพาะ API ที่จำเป็นก่อนนำไปใช้งานใน production

## License

ยังไม่ได้ระบุสัญญาอนุญาตใช้งาน (license) สำหรับโปรเจกต์นี้
