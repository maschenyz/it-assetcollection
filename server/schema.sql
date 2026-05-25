-- ============================================
-- Yuna Asset Management — Database Schema
-- Schema: asset | Run as postgres superuser
-- ============================================

DROP SCHEMA IF EXISTS asset CASCADE;
CREATE SCHEMA asset;

-- ============================================
-- 1. AUTH & USERS
-- ============================================
CREATE TABLE IF NOT EXISTS asset.users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'viewer', -- admin, operator, viewer
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMPTZ NULL
);

-- ============================================
-- 2. MASTER DATA
-- ============================================
CREATE TABLE IF NOT EXISTS asset.buildings (
    id SERIAL PRIMARY KEY,
    name VARCHAR(150) NOT NULL UNIQUE,
    code VARCHAR(20) DEFAULT ''
);

CREATE TABLE IF NOT EXISTS asset.departments (
    id SERIAL PRIMARY KEY,
    name VARCHAR(150) NOT NULL,
    code VARCHAR(20) DEFAULT '',
    building_id INTEGER REFERENCES asset.buildings(id) ON DELETE SET NULL,
    UNIQUE(name, building_id)
);

CREATE TABLE IF NOT EXISTS asset.locations (
    id SERIAL PRIMARY KEY,
    building_id INTEGER REFERENCES asset.buildings(id) ON DELETE SET NULL,
    department_id INTEGER REFERENCES asset.departments(id) ON DELETE SET NULL,
    room VARCHAR(100) DEFAULT ''
);

-- ============================================
-- 3. DEVICES (Core)
-- ============================================
CREATE TABLE IF NOT EXISTS asset.devices (
    uuid TEXT PRIMARY KEY,
    hostname TEXT NOT NULL DEFAULT '',
    device_type VARCHAR(20) DEFAULT 'PC',        -- PC, AIO, NB
    mac_address TEXT DEFAULT '',
    ip_address TEXT DEFAULT '',
    os_info JSONB DEFAULT '{}',
    windows_build TEXT DEFAULT '',
    bios_serial TEXT DEFAULT '',
    last_user TEXT DEFAULT '',
    location_id INTEGER REFERENCES asset.locations(id) ON DELETE SET NULL,
    allow_overtime BOOLEAN DEFAULT FALSE,
    status VARCHAR(20) DEFAULT 'OFFLINE',
    last_seen TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Migrate old devices table if columns missing
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS mac_address TEXT DEFAULT '';
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS windows_build TEXT DEFAULT '';
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS bios_serial TEXT DEFAULT '';
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS last_user TEXT DEFAULT '';
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS location_id INTEGER REFERENCES asset.locations(id) ON DELETE SET NULL;
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS allow_overtime BOOLEAN DEFAULT FALSE;
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS hostname TEXT DEFAULT '';

CREATE TABLE IF NOT EXISTS asset.hardware_inventory (
    device_id TEXT PRIMARY KEY REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    cpu_info JSONB DEFAULT '{}',
    ram_info JSONB DEFAULT '[]',
    storage_info JSONB DEFAULT '[]',
    gpu_info JSONB DEFAULT '{}',
    motherboard_info JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset.device_facts (
    device_id TEXT PRIMARY KEY REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    facts JSONB DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset.device_assets (
    device_id TEXT PRIMARY KEY REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    sku TEXT DEFAULT '',
    type_ict TEXT DEFAULT '',              -- ประเภทตาม ICT
    purchase_date DATE NULL,
    warranty_months INTEGER DEFAULT 0,
    warranty_expire DATE NULL,
    amount NUMERIC(12,2) DEFAULT 0,
    comment TEXT DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
    );

-- ============================================
-- 5. MONITORS
-- ============================================
CREATE TABLE IF NOT EXISTS asset.monitors (
    id SERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    serial TEXT DEFAULT '',
    model TEXT DEFAULT '',
    size_cm INTEGER DEFAULT 0,
    resolution TEXT DEFAULT '',
    manufacturer TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 4. SOFTWARE
-- ============================================
CREATE TABLE IF NOT EXISTS asset.software_catalog (
    id SERIAL PRIMARY KEY,
    software_name TEXT UNIQUE NOT NULL,
    publisher TEXT DEFAULT '',
    category TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS asset.software_installations (
    id SERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    software_id INTEGER REFERENCES asset.software_catalog(id) ON DELETE CASCADE,
    version TEXT DEFAULT '',
    install_path TEXT DEFAULT '',
    detected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(device_id, software_id)
);

CREATE TABLE IF NOT EXISTS asset.software_blacklist (
    id SERIAL PRIMARY KEY,
    software_name TEXT UNIQUE NOT NULL,
    reason TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 5. COMMANDS & TASK QUEUE
-- ============================================
CREATE TABLE IF NOT EXISTS asset.command_templates (
    id SERIAL PRIMARY KEY,
    cmd_name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    script_content TEXT NOT NULL,           -- หรือ special keyword: SCREENSHOT, SCAN_NETWORK
    category TEXT DEFAULT 'system',         -- system, maintenance, network, report
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset.task_queue (
    id SERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    cmd_template_id INTEGER REFERENCES asset.command_templates(id) ON DELETE SET NULL,
    custom_script TEXT DEFAULT '',
    status VARCHAR(20) DEFAULT 'pending',   -- pending, received, processing, success, failed, expired
    result_output TEXT DEFAULT '',
    created_by TEXT DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ DEFAULT (CURRENT_TIMESTAMP + INTERVAL '60 minutes'),
    received_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL
);

-- ============================================
-- 6. MONITORING & LOGS
-- ============================================
CREATE TABLE IF NOT EXISTS asset.heartbeats (
    id BIGSERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    ip_address TEXT DEFAULT '',
    cpu_pct FLOAT DEFAULT 0,
    ram_pct FLOAT DEFAULT 0,
    disk_pct FLOAT DEFAULT 0,
    captured_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset.change_logs (
    id BIGSERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    change_type VARCHAR(50) DEFAULT 'HARDWARE',
    field_name TEXT DEFAULT '',
    old_value TEXT DEFAULT '',
    new_value TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset.screenshots (
    id BIGSERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    image_data TEXT NOT NULL,               -- base64 JPEG
    task_id INTEGER REFERENCES asset.task_queue(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 7. PERIPHERALS
-- ============================================
CREATE TABLE IF NOT EXISTS asset.printers (
    id SERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE SET NULL,
    printer_name TEXT NOT NULL,
    brand TEXT DEFAULT '',
    printer_type VARCHAR(20) DEFAULT 'Laser',  -- Laser, Inkjet, Thermal
    model TEXT DEFAULT '',
    ip TEXT DEFAULT '',
    department_id INTEGER REFERENCES asset.departments(id) ON DELETE SET NULL,
    last_ink_change DATE NULL,
    total_pages_printed INT DEFAULT 0,
    starting_page_count INT DEFAULT 0,
    notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_device_printer UNIQUE (device_id, printer_name)
);

CREATE TABLE IF NOT EXISTS asset.usb_devices (
    id BIGSERIAL PRIMARY KEY,
    device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
    device_name TEXT DEFAULT '',
    device_type TEXT DEFAULT '',
    detected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- 8. SYSTEM SETTINGS
-- ============================================
CREATE TABLE IF NOT EXISTS asset.system_settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ============================================
-- INDEXES
-- ============================================
CREATE INDEX IF NOT EXISTS idx_devices_status ON asset.devices(status);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON asset.devices(last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_devices_location ON asset.devices(location_id);
CREATE INDEX IF NOT EXISTS idx_task_queue_device_status ON asset.task_queue(device_id, status);
CREATE INDEX IF NOT EXISTS idx_task_queue_status ON asset.task_queue(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_heartbeats_device ON asset.heartbeats(device_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_sw_install_device ON asset.software_installations(device_id);
CREATE INDEX IF NOT EXISTS idx_change_logs_device ON asset.change_logs(device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_screenshots_device ON asset.screenshots(device_id, created_at DESC);

-- ============================================
-- SEED DATA
-- ============================================

-- Default Command Templates
INSERT INTO asset.command_templates (cmd_name, description, script_content, category) VALUES
('screenshot',        'แคปหน้าจอ',                  'SCREENSHOT',                                    'report'),
('scan_network',      'สแกนวงแลนของเครื่องนี้',      'SCAN_NETWORK',                                  'network'),
('restart_spooler',   'รีสตาร์ท Print Spooler',      'net stop spooler && net start spooler',         'maintenance'),
('clear_temp',        'ล้างไฟล์ Temp',               'del /q /f /s "%TEMP%\*" & del /q /f /s "C:\Windows\Temp\*"', 'maintenance'),
('lock_screen',       'ล็อกหน้าจอ',                  'rundll32.exe user32.dll,LockWorkStation',       'system'),
('shutdown_60',       'ปิดเครื่องใน 60 วินาที',      'shutdown /s /f /t 60',                          'system'),
('shutdown_now',      'ปิดเครื่องทันที',              'shutdown /s /f /t 0',                           'system'),
('restart_now',       'รีสตาร์ทเครื่องทันที',        'shutdown /r /f /t 0',                           'system'),
('cancel_shutdown',   'ยกเลิกการปิดเครื่อง',         'shutdown /a',                                   'system'),
('flush_dns',         'ล้าง DNS Cache',               'ipconfig /flushdns',                            'network'),
('check_disk',        'ตรวจสอบ Disk Health',          'chkdsk C: /scan',                               'maintenance')
ON CONFLICT (cmd_name) DO NOTHING;

-- Default System Settings
INSERT INTO asset.system_settings (key, value) VALUES
('telegram_token',              ''),
('telegram_chat_id',            ''),
('line_token',                  ''),
('overtime_check_hour',         '20'),
('heartbeat_timeout_minutes',   '10'),
('task_expire_minutes',         '60'),
('report_title',                'รายงานสินทรัพย์ IT'),
('org_name',                    'โรงพยาบาล'),
('blacklist_alert_enabled',     'true')
ON CONFLICT (key) DO NOTHING;
