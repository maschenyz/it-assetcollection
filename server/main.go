package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

// --- Telegram Alert ---
func sendTelegram(msg string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		if db != nil {
			_ = db.QueryRow("SELECT value FROM asset.system_settings WHERE key = 'TELEGRAM_BOT_TOKEN'").Scan(&token)
			_ = db.QueryRow("SELECT value FROM asset.system_settings WHERE key = 'TELEGRAM_CHAT_ID'").Scan(&chatID)
		}
	}
	if token == "" || chatID == "" {
		fmt.Println("[Telegram] Token/ChatID not set, skipping alert")
		return
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id":    {chatID},
		"text":       {msg},
		"parse_mode": {"HTML"},
	})
	if err != nil {
		fmt.Println("[Telegram] Send error:", err)
		return
	}
	defer resp.Body.Close()
}

func sendTelegramToChat(chatID int64, msg string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" && db != nil {
		_ = db.QueryRow("SELECT value FROM asset.system_settings WHERE key = 'TELEGRAM_BOT_TOKEN'").Scan(&token)
	}
	if token == "" {
		return
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id":    {fmt.Sprintf("%d", chatID)},
		"text":       {msg},
		"parse_mode": {"HTML"},
	})
	if err == nil {
		resp.Body.Close()
	}
}

func handleTelegramCommand(chatID int64, text string) {
	cmd := strings.TrimSpace(strings.ToLower(text))
	if idx := strings.Index(cmd, "@"); idx != -1 {
		cmd = cmd[:idx]
	}

	switch cmd {
	case "/start", "/help":
		msg := `🏥 <b>Yuna Asset Bot — IT Cha-am Hospital</b> 🌸

ยินดีต้อนรับค่ะพี่แบงค์! ยูนะพร้อมช่วยรายงานและตรวจเช็คระบบ IT Asset ดังนี้ค่ะ:

📊 <b>/status</b> — สรุปสถานะเครื่องและอุปกรณ์ทั้งหมด
🚨 <b>/audit</b> — รายงานการแจ้งเตือนล่าสุด 5 รายการ
💻 <b>/list</b> — รายชื่อเครื่องคอมพิวเตอร์ที่ลงทะเบียนล่าสุด
❓ <b>/help</b> — ดูรายการคำสั่งทั้งหมด`
		sendTelegramToChat(chatID, msg)

	case "/status":
		var totalDev, onlineDev, offlineDev, printerCount, blCount int
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices").Scan(&totalDev)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices WHERE status = 'ONLINE'").Scan(&onlineDev)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices WHERE status = 'OFFLINE'").Scan(&offlineDev)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.printers").Scan(&printerCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.software_blacklist").Scan(&blCount)

		msg := fmt.Sprintf(`📊 <b>Yuna Asset — Executive Summary</b> 🌸

💻 <b>Computers:</b>
• รวมทั้งหมด: <b>%d</b> เครื่อง
• 🟢 Online: <b>%d</b> เครื่อง
• 🔴 Offline: <b>%d</b> เครื่อง

🖨 <b>Printers:</b> %d เครื่อง
🚫 <b>Blacklist Software:</b> %d รายการ`, totalDev, onlineDev, offlineDev, printerCount, blCount)
		sendTelegramToChat(chatID, msg)

	case "/audit":
		rows, err := db.Query(`
			SELECT d.hostname, l.change_type, l.field_name, COALESCE(l.new_value, ''), l.created_at
			FROM asset.change_logs l
			JOIN asset.devices d ON l.device_id = d.uuid
			WHERE l.change_type <> 'STATUS'
			ORDER BY l.created_at DESC LIMIT 5
		`)
		if err != nil {
			sendTelegramToChat(chatID, "❌ เกิดข้อผิดพลาดในการดึงข้อมูล Audit Logs ค่ะ")
			return
		}
		defer rows.Close()

		var lines []string
		for rows.Next() {
			var host, ctype, field, newv string
			var cat time.Time
			rows.Scan(&host, &ctype, &field, &newv, &cat)
			lines = append(lines, fmt.Sprintf("• <code>[%s]</code> <b>%s</b>: %s (%s)", cat.Format("15:04"), host, field, newv))
		}

		if len(lines) == 0 {
			sendTelegramToChat(chatID, "🚨 <b>System Audit Logs:</b> ยังไม่มีข้อมูลการแจ้งเตือนล่าสุดค่ะ")
		} else {
			msg := fmt.Sprintf("🚨 <b>Latest 5 Audit Logs:</b>\n\n%s", strings.Join(lines, "\n"))
			sendTelegramToChat(chatID, msg)
		}

	case "/list":
		rows, err := db.Query(`
			SELECT hostname, ip_address, status, COALESCE(last_user, 'N/A')
			FROM asset.devices
			ORDER BY last_seen DESC LIMIT 10
		`)
		if err != nil {
			sendTelegramToChat(chatID, "❌ เกิดข้อผิดพลาดในการดึงรายชื่อเครื่องค่ะ")
			return
		}
		defer rows.Close()

		var lines []string
		for rows.Next() {
			var host, ip, status, user string
			rows.Scan(&host, &ip, &status, &user)
			icon := "🟢"
			if status != "ONLINE" {
				icon = "🔴"
			}
			lines = append(lines, fmt.Sprintf("%s <b>%s</b> (%s) — User: %s", icon, host, ip, user))
		}

		msg := fmt.Sprintf("💻 <b>Registered Computers (Recent 10):</b>\n\n%s", strings.Join(lines, "\n"))
		sendTelegramToChat(chatID, msg)
	}
}

func startTelegramBot() {
	var offset int64 = 0
	for {
		time.Sleep(3 * time.Second)
		token := os.Getenv("TELEGRAM_BOT_TOKEN")
		if token == "" && db != nil {
			_ = db.QueryRow("SELECT value FROM asset.system_settings WHERE key = 'TELEGRAM_BOT_TOKEN'").Scan(&token)
		}
		if token == "" {
			continue
		}

		apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=2", token, offset)
		resp, err := http.Get(apiURL)
		if err != nil {
			continue
		}

		var result struct {
			Ok     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  *struct {
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
					Text string `json:"text"`
				} `json:"message"`
			} `json:"result"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Ok {
			for _, upd := range result.Result {
				offset = upd.UpdateID + 1
				if upd.Message != nil && upd.Message.Text != "" {
					handleTelegramCommand(upd.Message.Chat.ID, upd.Message.Text)
				}
			}
		}
		resp.Body.Close()
	}
}

func logChange(deviceID, ctype, field, oldV, newV string) {
	db.Exec(`INSERT INTO asset.change_logs (device_id, change_type, field_name, old_value, new_value)
	         VALUES ($1, $2, $3, $4, $5)`, deviceID, ctype, field, oldV, newV)
}

// --- Models ---
type AgentCheckin struct {
	UUID         string                   `json:"uuid"`
	Hostname     string                   `json:"hostname"`
	DeviceType   string                   `json:"device_type"`
	IPAddress    string                   `json:"ip_address"`
	MACAddress   string                   `json:"mac_address"`
	OSInfo       map[string]interface{}   `json:"os_info"`
	WindowsBuild string                   `json:"windows_build"`
	BiosSerial   string                   `json:"bios_serial"`
	LastUser     string                   `json:"last_user"`
	Hardware     map[string]interface{}   `json:"hardware"`
	SoftwareList []map[string]interface{} `json:"software_list"`
	Printers     []map[string]interface{} `json:"printers"`
	USBDevices   []map[string]interface{} `json:"usb_devices"`
	Monitors     []map[string]interface{} `json:"monitors"`
	Stats        map[string]interface{}   `json:"stats"` // cpu_pct, ram_pct, disk_pct
	Facts        map[string]interface{}   `json:"facts"` // extra agent facts (hotfixes, defender, services, etc.)
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// --- Helpers ---
func cleanString(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
}

func toJSON(data interface{}) []byte {
	b, _ := json.Marshal(data)
	return b
}

func upsertSecurityProducts(deviceID string, facts map[string]interface{}) {
	rawProducts, ok := facts["security_products"].([]interface{})
	if !ok {
		return // Older agents do not send this field yet.
	}
	for _, raw := range rawProducts {
		product, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name := cleanString(fmt.Sprint(product["name"]))
		if name == "" || name == "<nil>" {
			continue
		}
		vendor := cleanString(fmt.Sprint(product["vendor"]))
		source := cleanString(fmt.Sprint(product["source"]))
		signatureStatus := cleanString(fmt.Sprint(product["signature_status"]))
		realtime, _ := product["realtime_protection_enabled"].(bool)
		state := 0
		switch value := product["product_state"].(type) {
		case float64:
			state = int(value)
		case int:
			state = value
		}
		_, err := db.Exec(`INSERT INTO asset.security_products
			(device_id, product_name, vendor, product_state, realtime_protection_enabled, signature_status, source, raw_data, detected_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP)
			ON CONFLICT (device_id, product_name) DO UPDATE SET
			vendor = EXCLUDED.vendor, product_state = EXCLUDED.product_state,
			realtime_protection_enabled = EXCLUDED.realtime_protection_enabled,
			signature_status = EXCLUDED.signature_status, source = EXCLUDED.source,
			raw_data = EXCLUDED.raw_data, detected_at = CURRENT_TIMESTAMP`,
			deviceID, name, vendor, state, realtime, signatureStatus, source, toJSON(product))
		if err != nil {
			log.Printf("[Security] upsert error device=%s product=%q: %v", deviceID, name, err)
		}
	}
}

func normalizeBrand(b string) string {
	b = strings.TrimSpace(b)
	if b == "" || strings.ToLower(b) == "<nil>" || strings.ToLower(b) == "unknown" {
		return "Unknown"
	}
	bLower := strings.ToLower(b)
	if strings.Contains(bLower, "hewlett-packard") || bLower == "hp" {
		return "HP"
	}
	if strings.Contains(bLower, "lenovo") {
		return "Lenovo"
	}
	if strings.Contains(bLower, "asustek") || strings.Contains(bLower, "asus") {
		return "ASUS"
	}
	if strings.Contains(bLower, "micro-star") || bLower == "msi" {
		return "MSI"
	}
	if strings.Contains(bLower, "dell") {
		return "Dell"
	}
	if strings.Contains(bLower, "gigabyte") {
		return "Gigabyte"
	}
	if strings.Contains(bLower, "apple") {
		return "Apple"
	}
	if strings.Contains(bLower, "acer") {
		return "Acer"
	}
	if strings.Contains(bLower, "samsung") {
		return "Samsung"
	}
	// Capitalize first letter
	return strings.ToUpper(b[:1]) + b[1:]
}

// --- Monitors ---
type MonitorInfo struct {
	ID           int    `json:"id"`
	DeviceID     string `json:"device_id"`
	Serial       string `json:"serial"`
	Model        string `json:"model"`
	SizeInch     int    `json:"size_inch"`
	SizeCm       int    `json:"size_cm"`
	Resolution   string `json:"resolution"`
	Manufacturer string `json:"manufacturer"`
	CreatedAt    string `json:"created_at"`
}

// upsertMonitors handles saving monitor data from agent checkin payload into the DB.
// It deletes all existing monitors for the device then re-inserts (simpler than
// per-row upsert given monitors rarely change and serials can be empty/N/A).
func upsertMonitors(deviceID string, monitors []map[string]interface{}) {
	if len(monitors) == 0 {
		return
	}

	_, err := db.Exec(`DELETE FROM asset.monitors WHERE device_id = $1`, deviceID)
	if err != nil {
		log.Printf("[Monitor] Failed to clear old monitors for %s: %v", deviceID, err)
		return
	}

	for _, m := range monitors {
		serial := cleanString(fmt.Sprintf("%v", m["serial"]))
		model := cleanString(fmt.Sprintf("%v", m["model"]))
		manufacturer := cleanString(fmt.Sprintf("%v", m["manufacturer"]))
		resolution := cleanString(fmt.Sprintf("%v", m["resolution"]))
		if model == "" || model == "<nil>" {
			continue
		}
		if serial == "<nil>" {
			serial = "N/A"
		}
		if manufacturer == "<nil>" {
			manufacturer = ""
		}
		if resolution == "<nil>" {
			resolution = ""
		}

		size_inch := 0
		if v, ok := m["size_inch"]; ok {
			switch val := v.(type) {
			case float64:
				size_inch = int(val)
			case int:
				size_inch = val
			}
		}

		_, err := db.Exec(`
			INSERT INTO asset.monitors (device_id, serial, model, size_inch, resolution, manufacturer)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, deviceID, serial, model, size_inch, resolution, manufacturer)
		if err != nil {
			log.Printf("[Monitor] Insert error for device %s, model %s: %v", deviceID, model, err)
		}
	}
	log.Printf("[Monitor] Upserted %d monitor(s) for device %s", len(monitors), deviceID)
}

func getMonitors(c *fiber.Ctx) error {
	uuid := c.Params("uuid")
	if uuid == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "uuid required"})
	}
	rows, err := db.Query(`
		SELECT id, device_id, serial, model, size_inch, resolution, manufacturer, created_at
		FROM asset.monitors
		WHERE device_id = $1
		ORDER BY id`, uuid)
	if err != nil {
		log.Printf("[Monitor] DB query error: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	defer rows.Close()

	var monitors []MonitorInfo
	for rows.Next() {
		var m MonitorInfo
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.Serial, &m.Model,
			&m.SizeInch, &m.Resolution, &m.Manufacturer, &m.CreatedAt); err != nil {
			log.Printf("[Monitor] scan error: %v", err)
			continue
		}
		monitors = append(monitors, m)
	}
	if monitors == nil {
		monitors = []MonitorInfo{}
	}
	return c.JSON(monitors)
}

func registerMonitorRoutes(app *fiber.App) {
	v1 := app.Group("/api/v1")
	v1.Get("/devices/:uuid/monitors", verifyAdmin, getMonitors)
}

func classifyDeviceType(mfg, product, hostname, biosSerial string) string {
	mfgLower := strings.ToLower(mfg)
	prodLower := strings.ToLower(product)
	hostLower := strings.ToLower(hostname)

	// VM Check
	if strings.Contains(mfgLower, "vmware") ||
		strings.Contains(mfgLower, "virtualbox") ||
		strings.Contains(prodLower, "virtual machine") ||
		strings.Contains(prodLower, "vmware") ||
		strings.Contains(hostLower, "vm-") ||
		strings.HasPrefix(strings.ToLower(biosSerial), "vmware") {
		return "Virtual Machine (VM)"
	}

	// AIO Check
	if strings.Contains(prodLower, "aio") ||
		strings.Contains(prodLower, "all-in-one") ||
		strings.Contains(prodLower, "all in one") {
		return "All-in-One (AIO)"
	}

	// Laptop Check
	if strings.Contains(prodLower, "laptop") ||
		strings.Contains(prodLower, "notebook") ||
		strings.Contains(prodLower, "elitebook") ||
		strings.Contains(prodLower, "probook") ||
		strings.Contains(prodLower, "thinkpad") ||
		strings.Contains(prodLower, "latitude") ||
		strings.Contains(prodLower, "inspiron") ||
		strings.Contains(prodLower, "vostro") ||
		strings.Contains(prodLower, "zenbook") ||
		strings.Contains(prodLower, "vivobook") ||
		strings.Contains(hostLower, "laptop-") ||
		strings.Contains(prodLower, "macbook") {
		return "Laptop / Notebook"
	}

	return "Desktop PC"
}

func classifyUSBDevice(name string) string {
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "printer") ||
		strings.Contains(nameLower, "print") ||
		strings.Contains(nameLower, "pantum") ||
		strings.Contains(nameLower, "canon") ||
		strings.Contains(nameLower, "epson") ||
		strings.Contains(nameLower, "brother") ||
		strings.Contains(nameLower, "hp ") ||
		strings.Contains(nameLower, "laserjet") ||
		strings.Contains(nameLower, "smart tank") ||
		strings.Contains(nameLower, "label printer") ||
		strings.Contains(nameLower, "zebra") ||
		strings.Contains(nameLower, "oki") ||
		strings.Contains(nameLower, "samsung") {
		return "Printer"
	}
	if strings.Contains(nameLower, "storage") ||
		strings.Contains(nameLower, "flash") ||
		strings.Contains(nameLower, "cruzer") ||
		strings.Contains(nameLower, "sandisk") ||
		strings.Contains(nameLower, "kingston") ||
		strings.Contains(nameLower, "transcend") ||
		strings.Contains(nameLower, "wd ") ||
		strings.Contains(nameLower, "seagate") ||
		strings.Contains(nameLower, "hard drive") ||
		strings.Contains(nameLower, "drive") ||
		strings.Contains(nameLower, "card reader") {
		return "Storage Device"
	}
	if strings.Contains(nameLower, "keyboard") ||
		strings.Contains(nameLower, "mouse") ||
		strings.Contains(nameLower, "pointing") ||
		strings.Contains(nameLower, "gamepad") ||
		strings.Contains(nameLower, "joystick") ||
		strings.Contains(nameLower, "receiver") {
		return "Input Device"
	}
	if strings.Contains(nameLower, "camera") ||
		strings.Contains(nameLower, "webcam") ||
		strings.Contains(nameLower, "video") {
		return "Imaging Device"
	}
	if strings.Contains(nameLower, "audio") ||
		strings.Contains(nameLower, "speaker") ||
		strings.Contains(nameLower, "headset") ||
		strings.Contains(nameLower, "microphone") ||
		strings.Contains(nameLower, "sound") {
		return "Audio Device"
	}
	if strings.Contains(nameLower, "bluetooth") ||
		strings.Contains(nameLower, "wireless") ||
		strings.Contains(nameLower, "ethernet") ||
		strings.Contains(nameLower, "network") ||
		strings.Contains(nameLower, "lan") {
		return "Network Device"
	}
	return "External Device"
}

// --- Middleware ---
func verifyAgent(c *fiber.Ctx) error {
	token := c.Get("X-Agent-Token")
	if token != os.Getenv("AGENT_SECRET_TOKEN") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Agent Token"})
	}
	return c.Next()
}

func verifyAdmin(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	var tokenStr string
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		tokenStr = c.Query("token")
	}

	if tokenStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Session"})
	}

	claims := token.Claims.(jwt.MapClaims)
	c.Locals("user", claims["sub"])
	return c.Next()
}

func main() {
	// 1. Load ENV
	_ = godotenv.Load()
	if os.Getenv("DB_HOST") == "" {
		_ = godotenv.Load("server/.env")
	}
	if os.Getenv("DB_HOST") == "" {
		_ = godotenv.Load("C:/project A/server/.env")
	}

	// 2. Database Setup (Built-in Connection Pool)
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable timezone=Asia/Bangkok",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Critical Error: Could not connect to database: %v", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)

	// --- 2.1 Auto Migration ---
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS brand TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS printer_type TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS model TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS ip TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS total_pages_printed INT DEFAULT 0;")
	db.Exec("ALTER TABLE asset.devices ADD COLUMN IF NOT EXISTS asset_number TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.hardware_inventory ADD COLUMN IF NOT EXISTS bios_info JSONB DEFAULT '{}';")
	// Ensure ON CONFLICT (device_id, printer_name) works even on older schemas
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_printers_device_name_unique ON asset.printers (device_id, printer_name);")
	db.Exec("ALTER TABLE asset.printers ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP;")
	db.Exec("ALTER TABLE asset.task_queue ADD COLUMN IF NOT EXISTS result_output TEXT DEFAULT '';")
	db.Exec("ALTER TABLE asset.task_queue ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL;")
	db.Exec(`CREATE TABLE IF NOT EXISTS asset.printer_logs (
		id SERIAL PRIMARY KEY, 
		printer_id INTEGER REFERENCES asset.printers(id) ON DELETE CASCADE, 
		event_type TEXT, 
		description TEXT, 
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);`)
	db.Exec(`CREATE TABLE IF NOT EXISTS asset.monitors (
		id SERIAL PRIMARY KEY,
		device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
		serial TEXT DEFAULT '',
		model TEXT DEFAULT '',
		size_inch INTEGER DEFAULT 0,
		resolution TEXT DEFAULT '',
		manufacturer TEXT DEFAULT '',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);`)
	db.Exec(`CREATE TABLE IF NOT EXISTS asset.device_facts (
		device_id TEXT PRIMARY KEY REFERENCES asset.devices(uuid) ON DELETE CASCADE,
		facts JSONB DEFAULT '{}'::jsonb,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);`)
	db.Exec(`CREATE TABLE IF NOT EXISTS asset.security_products (
		id BIGSERIAL PRIMARY KEY,
		device_id TEXT REFERENCES asset.devices(uuid) ON DELETE CASCADE,
		product_name TEXT NOT NULL,
		vendor TEXT DEFAULT '',
		product_state INTEGER DEFAULT 0,
		realtime_protection_enabled BOOLEAN DEFAULT FALSE,
		signature_status TEXT DEFAULT 'unknown',
		source TEXT DEFAULT '',
		raw_data JSONB DEFAULT '{}'::jsonb,
		detected_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(device_id, product_name)
	);`)
	db.Exec("CREATE INDEX IF NOT EXISTS idx_security_products_device ON asset.security_products(device_id, detected_at DESC);")

	// 3. Init Fiber App
	app := fiber.New(fiber.Config{
		AppName: "Yuna Asset Management v3.0",
	})
	app.Use(cors.New())

	// --- 4. API Endpoints ---
	v1 := app.Group("/api/v1")
	registerMonitorRoutes(app)

	// Agent Checkin
	v1.Post("/agent/checkin", verifyAgent, func(c *fiber.Ctx) error {
		var payload AgentCheckin
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Bad Request"})
		}

		hostname := cleanString(payload.Hostname)
		biosSerial := cleanString(payload.BiosSerial)
		lastUser := cleanString(payload.LastUser)
		ipAddress := cleanString(payload.IPAddress)

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer tx.Rollback()

		var isNewDevice bool
		_ = tx.QueryRow("SELECT NOT EXISTS(SELECT 1 FROM asset.devices WHERE uuid = $1)", payload.UUID).Scan(&isNewDevice)

		// 1. Update Device Info
		_, err = tx.Exec(`
			INSERT INTO asset.devices (uuid, hostname, device_type, mac_address, ip_address, os_info, windows_build, bios_serial, last_user, status, last_seen)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'ONLINE', CURRENT_TIMESTAMP)
			ON CONFLICT (uuid) DO UPDATE SET
				hostname = EXCLUDED.hostname, ip_address = EXCLUDED.ip_address,
				os_info = EXCLUDED.os_info, windows_build = EXCLUDED.windows_build,
				bios_serial = EXCLUDED.bios_serial, last_user = EXCLUDED.last_user,
				status = 'ONLINE', last_seen = CURRENT_TIMESTAMP;
		`, payload.UUID, hostname, payload.DeviceType, payload.MACAddress, ipAddress, toJSON(payload.OSInfo), payload.WindowsBuild, biosSerial, lastUser)
		if err != nil {
			log.Printf("[AgentCheckin] devices upsert error uuid=%s: %v", payload.UUID, err)
			return c.Status(500).SendString(err.Error())
		}

		// 2. Hardware Inventory
		_, err = tx.Exec(`
			INSERT INTO asset.hardware_inventory (device_id, cpu_info, ram_info, storage_info, gpu_info, motherboard_info, bios_info, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
			ON CONFLICT (device_id) DO UPDATE SET
				cpu_info = EXCLUDED.cpu_info, ram_info = EXCLUDED.ram_info,
				storage_info = EXCLUDED.storage_info, gpu_info = EXCLUDED.gpu_info,
				motherboard_info = EXCLUDED.motherboard_info, bios_info = EXCLUDED.bios_info,
				updated_at = CURRENT_TIMESTAMP;
		`, payload.UUID, toJSON(payload.Hardware["cpu"]), toJSON(payload.Hardware["ram"]), toJSON(payload.Hardware["storage"]), toJSON(payload.Hardware["gpu"]), toJSON(payload.Hardware["motherboard"]), toJSON(payload.Hardware["bios"]))
		if err != nil {
			log.Printf("[AgentCheckin] hardware_inventory upsert error uuid=%s: %v", payload.UUID, err)
			return c.Status(500).SendString(err.Error())
		}

		// 2.2 S.M.A.R.T. Predict Failure Detection — alert once per newly failing disk
		if storageList, ok := payload.Hardware["storage"].([]interface{}); ok {
			for _, rawDisk := range storageList {
				disk, ok := rawDisk.(map[string]interface{})
				if !ok {
					continue
				}
				predictFail, _ := disk["predict_failure"].(bool)
				if !predictFail {
					continue
				}

				diskModel := cleanString(fmt.Sprintf("%v", disk["model"]))
				diskSerial := cleanString(fmt.Sprintf("%v", disk["serial"]))

				// Check if this disk was already flagged in the previous sync (avoid spam)
				var alreadyFlagged bool
				_ = db.QueryRow(`
					SELECT EXISTS(
						SELECT 1 FROM asset.hardware_inventory
						WHERE device_id = $1
						  AND storage_info @> $2::jsonb
					)`, payload.UUID,
					fmt.Sprintf(`[{"serial": "%s", "predict_failure": true}]`, diskSerial),
				).Scan(&alreadyFlagged)

				if !alreadyFlagged {
					log.Printf("[SMART] Disk predict failure on %s: %s (S/N: %s)", payload.Hostname, diskModel, diskSerial)
					msg := fmt.Sprintf(
						"🚨 <b>Disk Failure Alert!</b>\n🖥 Host: <b>%s</b>\n💿 Disk: <code>%s</code>\n🔢 Serial: <code>%s</code>\n\n⚠️ S.M.A.R.T. ตรวจพบว่าดิสก์นี้กำลังจะพัง! <b>กรุณาสำรองข้อมูลและเปลี่ยน SSD โดยเร็วค่ะ!</b>",
						payload.Hostname, diskModel, diskSerial,
					)
					go sendTelegram(msg)
					logChange(payload.UUID, "HARDWARE", "Disk SMART", "OK", "PREDICT FAIL: "+diskModel)
				}
			}
		}

		// 2.1 Extra Agent Facts (store for after-commit best-effort)
		var factsJSON []byte
		if payload.Facts != nil {
			factsJSON = toJSON(payload.Facts)
		}

		// 3. Heartbeat
		cpuPct := payload.Stats["cpu_pct"]
		ramPct := payload.Stats["ram_pct"]
		diskPct := payload.Stats["disk_pct"]
		_, err = tx.Exec(`INSERT INTO asset.heartbeats (device_id, ip_address, cpu_pct, ram_pct, disk_pct) VALUES ($1, $2, $3, $4, $5)`,
			payload.UUID, payload.IPAddress, cpuPct, ramPct, diskPct)
		if err != nil {
			log.Printf("[AgentCheckin] heartbeats insert error uuid=%s: %v", payload.UUID, err)
			return c.Status(500).SendString(err.Error())
		}

		// 4. Software Update & Blacklist Detection
		var blacklist []string
		blRows, _ := db.Query("SELECT software_name FROM asset.software_blacklist")
		if blRows != nil {
			defer blRows.Close()
			for blRows.Next() {
				var bn string
				blRows.Scan(&bn)
				blacklist = append(blacklist, strings.ToLower(bn))
			}
		}

		for _, sw := range payload.SoftwareList {
			swName := cleanString(fmt.Sprintf("%v", sw["name"]))
			if swName == "" || swName == "<nil>" {
				continue
			}
			swVersion := cleanString(fmt.Sprintf("%v", sw["version"]))
			swPath := cleanString(fmt.Sprintf("%v", sw["install_path"]))

			var swID int
			err = tx.QueryRow(`
				WITH ins AS (
					INSERT INTO asset.software_catalog (software_name) VALUES ($1)
					ON CONFLICT (software_name) DO NOTHING RETURNING id
				)
				SELECT id FROM ins UNION ALL SELECT id FROM asset.software_catalog WHERE software_name = $1 LIMIT 1
			`, swName).Scan(&swID)

			if err == nil {
				var exists bool
				if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM asset.software_installations WHERE device_id=$1 AND software_id=$2)", payload.UUID, swID).Scan(&exists); err != nil {
					log.Printf("[AgentCheckin] software_installations exists query error uuid=%s sw=%q: %v", payload.UUID, swName, err)
					return c.Status(500).SendString(err.Error())
				}
				if !exists {
					logChange(payload.UUID, "SOFTWARE", "Installed", "", swName+" ("+swVersion+")")

					var isBlacklisted bool
					for _, b := range blacklist {
						if strings.Contains(strings.ToLower(swName), b) {
							isBlacklisted = true
							msg := fmt.Sprintf("🚨 <b>Blacklist Alert!</b>\n🖥 Host: <b>%s</b>\n📦 Program: <code>%s</code>\n👤 User: %s",
								payload.Hostname, swName, payload.LastUser)
							go sendTelegram(msg)
							break
						}
					}

					if !isBlacklisted {
						msg := fmt.Sprintf("📦 <b>New Program Installed!</b>\n🖥 Host: <b>%s</b>\n📦 Program: <code>%s</code> (v%s)\n👤 User: %s",
							payload.Hostname, swName, swVersion, payload.LastUser)
						go sendTelegram(msg)
					}
				}

				_, err = tx.Exec(`
					INSERT INTO asset.software_installations (device_id, software_id, version, install_path)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (device_id, software_id) DO UPDATE SET
						version = EXCLUDED.version, install_path = EXCLUDED.install_path, detected_at = CURRENT_TIMESTAMP
				`, payload.UUID, swID, swVersion, swPath)
				if err != nil {
					log.Printf("[AgentCheckin] software_installations upsert error uuid=%s sw=%q: %v", payload.UUID, swName, err)
					return c.Status(500).SendString(err.Error())
				}
			}
		}

		// 5. USB Devices Update & Logging
		for _, usb := range payload.USBDevices {
			usbName := cleanString(fmt.Sprintf("%v", usb["name"]))
			if usbName == "" {
				continue
			}

			var exists bool
			if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM asset.usb_devices WHERE device_id=$1 AND device_name=$2)", payload.UUID, usbName).Scan(&exists); err != nil {
				log.Printf("[AgentCheckin] usb_devices exists query error uuid=%s usb=%q: %v", payload.UUID, usbName, err)
				return c.Status(500).SendString(err.Error())
			}
			if !exists {
				devType := classifyUSBDevice(usbName)
				logChange(payload.UUID, "USB", "Plugged", "", usbName+" ("+devType+")")
				if _, err := tx.Exec(
					"INSERT INTO asset.usb_devices (device_id, device_name, device_type) VALUES ($1, $2, $3)",
					payload.UUID, usbName, devType,
				); err != nil {
					log.Printf("[AgentCheckin] usb_devices insert error uuid=%s usb=%q: %v", payload.UUID, usbName, err)
					return c.Status(500).SendString(err.Error())
				}

				msg := fmt.Sprintf("🔌 <b>New USB Connected!</b>\n🖥 Host: <b>%s</b>\n💾 Device: <code>%s</code>\n🏷 Type: %s\n👤 User: %s",
					payload.Hostname, usbName, devType, payload.LastUser)
				go sendTelegram(msg)
			}
		}

		// 6. Printers Update & Logging — use top-level payload.Printers
		// Printers are NEVER deleted when not in checkin; last_seen_at tracks staleness.
		if len(payload.Printers) > 0 {
			for _, pMap := range payload.Printers {
				name := cleanString(fmt.Sprintf("%v", pMap["name"]))
				if name == "" || name == "<nil>" {
					continue
				}
				model := cleanString(fmt.Sprintf("%v", pMap["model"]))
				if model == "<nil>" {
					model = ""
				}
				port := cleanString(fmt.Sprintf("%v", pMap["port"]))
				if port == "<nil>" {
					port = ""
				}
				brand := cleanString(fmt.Sprintf("%v", pMap["manufacturer"]))
				if brand == "<nil>" {
					brand = ""
				}

				var jobs int
				if jobsRaw, ok := pMap["total_pages_printed"]; ok {
					if jNum, ok := jobsRaw.(float64); ok {
						jobs = int(jNum)
					}
				}

				pType := "Virtual"
				isNetwork := false
				if netVal, ok := pMap["network"].(bool); ok {
					isNetwork = netVal
				}

				nameLower := strings.ToLower(name)
				modelLower := strings.ToLower(model)

				isPhysicalUSB := false
				for _, usb := range payload.USBDevices {
					usbName := strings.ToLower(cleanString(fmt.Sprintf("%v", usb["name"])))
					if usbName == "" {
						continue
					}
					if strings.Contains(nameLower, usbName) || strings.Contains(usbName, nameLower) ||
						(modelLower != "" && (strings.Contains(modelLower, usbName) || strings.Contains(usbName, modelLower))) {
						isPhysicalUSB = true
						break
					}
				}

				if isPhysicalUSB {
					pType = "USB"
				} else if isNetwork || strings.HasPrefix(strings.ToLower(port), "http") || strings.HasPrefix(strings.ToLower(port), "ipp") || strings.HasPrefix(strings.ToLower(port), "10.") || strings.HasPrefix(strings.ToLower(port), "192.168.") || strings.HasPrefix(port, `\\`) {
					pType = "Network"
				} else if strings.Contains(nameLower, "pdf") || strings.Contains(nameLower, "xps") || strings.Contains(nameLower, "onenote") || strings.Contains(nameLower, "writer") || strings.Contains(nameLower, "fax") || strings.Contains(nameLower, "snagit") {
					pType = "Virtual"
				} else if strings.HasPrefix(strings.ToUpper(port), "USB") {
					pType = "USB (Disconnected)"
				} else {
					pType = "Local"
				}

				var oldTotal int
				var printerID int
				if err := tx.QueryRow(
					"SELECT COALESCE(MIN(id), 0), COALESCE(MAX(total_pages_printed), 0) FROM asset.printers WHERE device_id = $1 AND LOWER(TRIM(printer_name)) = LOWER(TRIM($2))",
					payload.UUID, name,
				).Scan(&printerID, &oldTotal); err != nil {
					log.Printf("[AgentCheckin] printers select error uuid=%s printer=%q: %v", payload.UUID, name, err)
					return c.Status(500).SendString(err.Error())
				}

				if printerID > 0 {
					_, err = tx.Exec(`
						UPDATE asset.printers SET
							printer_name = $2,
							brand = CASE WHEN $3 <> '' THEN $3 ELSE brand END,
							model = CASE WHEN $4 <> '' THEN $4 ELSE model END,
							ip = CASE WHEN $5 <> '' THEN $5 ELSE ip END,
							printer_type = $6,
							total_pages_printed = $7,
							last_seen_at = CURRENT_TIMESTAMP
						WHERE id = $1
					`, printerID, name, brand, model, port, pType, jobs)
					if err != nil {
						log.Printf("[AgentCheckin] printers update error uuid=%s printer=%q: %v", payload.UUID, name, err)
						return c.Status(500).SendString(err.Error())
					}

					// Cleanup duplicate secondary printer rows for same device & name
					_, _ = tx.Exec(`
						DELETE FROM asset.printers 
						WHERE device_id = $1 AND LOWER(TRIM(printer_name)) = LOWER(TRIM($2)) AND id <> $3
					`, payload.UUID, name, printerID)
				} else {
					err = tx.QueryRow(`
						INSERT INTO asset.printers (device_id, printer_name, brand, model, ip, printer_type, total_pages_printed, last_seen_at)
						VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
						RETURNING id
					`, payload.UUID, name, brand, model, port, pType, jobs).Scan(&printerID)
					if err != nil {
						log.Printf("[AgentCheckin] printers insert error uuid=%s printer=%q: %v", payload.UUID, name, err)
						return c.Status(500).SendString(err.Error())
					}
				}

				if jobs > oldTotal && jobs > 0 && printerID > 0 {
					if _, err := tx.Exec(
						"INSERT INTO asset.printer_logs (printer_id, event_type, description) VALUES ($1, $2, $3)",
						printerID, "PAGE_COUNT_UPDATE", fmt.Sprintf("Total pages printed: %d (Printed: +%d pages)", jobs, jobs-oldTotal),
					); err != nil {
						log.Printf("[AgentCheckin] printer_logs insert error uuid=%s printer_id=%d: %v", payload.UUID, printerID, err)
						return c.Status(500).SendString(err.Error())
					}
				}
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("[AgentCheckin] commit error uuid=%s: %v", payload.UUID, err)
			return c.Status(500).SendString(err.Error())
		}

		if isNewDevice {
			msg := fmt.Sprintf("✨ <b>New Computer Registered!</b>\n🖥 Host: <b>%s</b>\n🌐 IP: <code>%s</code>\n💻 Type: %s\n👤 User: %s",
				hostname, ipAddress, payload.DeviceType, lastUser)
			go sendTelegram(msg)
			logChange(payload.UUID, "DEVICE", "Registration", "", "New Device Connected")
		}

		// 6.1 Save extra facts outside transaction (facts should never break inventory sync)
		if len(factsJSON) > 0 {
			if _, err := db.Exec(`
				INSERT INTO asset.device_facts (device_id, facts, updated_at)
				VALUES ($1, $2, CURRENT_TIMESTAMP)
				ON CONFLICT (device_id) DO UPDATE SET
					facts = EXCLUDED.facts,
					updated_at = CURRENT_TIMESTAMP;
			`, payload.UUID, factsJSON); err != nil {
				log.Printf("[AgentCheckin] device_facts upsert error uuid=%s: %v", payload.UUID, err)
			}
		}
		upsertSecurityProducts(payload.UUID, payload.Facts)

		// 7. Monitors (outside tx — full replace strategy)
		go upsertMonitors(payload.UUID, payload.Monitors)

		return c.JSON(fiber.Map{"status": "ok", "timestamp": time.Now().Format(time.RFC3339)})
	})

	// GET /agent/tasks — Agent polls for pending tasks
	v1.Get("/agent/tasks", verifyAgent, func(c *fiber.Ctx) error {
		uuid := c.Query("uuid")
		if uuid == "" {
			return c.Status(400).JSON(fiber.Map{"error": "uuid parameter required"})
		}

		rows, err := db.Query(`
			SELECT id, COALESCE(custom_script, '') as command_type
			FROM asset.task_queue
			WHERE device_id = $1 AND status = 'pending'
			ORDER BY id ASC
		`, uuid)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var tasks []fiber.Map
		var taskIDs []int
		for rows.Next() {
			var id int
			var cmdType string
			rows.Scan(&id, &cmdType)
			tasks = append(tasks, fiber.Map{
				"id":           id,
				"command_type": cmdType,
			})
			taskIDs = append(taskIDs, id)
		}

		// Update polled tasks status to 'processing'
		for _, tid := range taskIDs {
			_, _ = db.Exec("UPDATE asset.task_queue SET status = 'processing', received_at = CURRENT_TIMESTAMP WHERE id = $1", tid)
		}

		if tasks == nil {
			tasks = []fiber.Map{}
		}
		return c.JSON(tasks)
	})

	// POST /agent/tasks/:id/result — Agent reports command execution status & log output
	v1.Post("/agent/tasks/:id/result", verifyAgent, func(c *fiber.Ctx) error {
		id := c.Params("id")
		var req struct {
			Status string `json:"status"`
			Output string `json:"output"`
			Result string `json:"result"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Bad Request")
		}

		finalOutput := req.Output
		if finalOutput == "" {
			finalOutput = req.Result
		}

		finalStatus := strings.ToLower(req.Status)
		if finalStatus != "success" && finalStatus != "failed" {
			finalStatus = "completed"
		}

		_, err := db.Exec(`
			UPDATE asset.task_queue
			SET status = $1, result_output = $2, completed_at = CURRENT_TIMESTAMP
			WHERE id = $3
		`, finalStatus, finalOutput, id)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// POST /agent/screenshot — Agent uploads remote screenshot (base64)
	v1.Post("/agent/screenshot", verifyAgent, func(c *fiber.Ctx) error {
		uuid := c.Query("uuid")
		taskIDStr := c.Query("task_id")

		bodyBytes := c.Body()
		if len(bodyBytes) == 0 {
			return c.Status(400).SendString("Empty image payload")
		}
		imgData := string(bodyBytes)

		var taskID *int
		if taskIDStr != "" {
			if tid, err := strconv.Atoi(taskIDStr); err == nil {
				taskID = &tid
			}
		}

		_, err := db.Exec(`
			INSERT INTO asset.screenshots (device_id, image_data, task_id)
			VALUES ($1, $2, $3)
		`, uuid, imgData, taskID)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// --- 5. Dashboard Auth & Stats ---
	v1.Post("/auth/login", func(c *fiber.Ctx) error {
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Bad Request")
		}

		var hash, role string
		err := db.QueryRow("SELECT password_hash, role FROM asset.users WHERE username = $1 AND is_active = TRUE", req.Username).Scan(&hash, &role)

		if err == sql.ErrNoRows {
			// Fallback .env Admin
			if req.Username == os.Getenv("ADMIN_USERNAME") && req.Password == os.Getenv("ADMIN_PASSWORD") {
				role = "admin"
			} else {
				return c.Status(401).JSON(fiber.Map{"detail": "Invalid credentials"})
			}
		} else if err == nil {
			if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
				return c.Status(401).JSON(fiber.Map{"detail": "Invalid credentials"})
			}
		}

		claims := jwt.MapClaims{
			"sub":  req.Username,
			"role": role,
			"exp":  time.Now().Add(time.Hour * 12).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		t, _ := token.SignedString([]byte(os.Getenv("JWT_SECRET")))

		return c.JSON(fiber.Map{"access_token": t, "token_type": "bearer", "role": role})
	})

	// GET /dashboard/metrics — Dashboard statistics
	v1.Get("/dashboard/metrics", verifyAdmin, func(c *fiber.Ctx) error {
		// 1. KPIs
		var computers, online, offline, monitors, printers, software, licenses int

		db.QueryRow("SELECT count(*) FROM asset.devices").Scan(&computers)
		db.QueryRow("SELECT count(*) FROM asset.devices WHERE status = 'ONLINE'").Scan(&online)
		db.QueryRow("SELECT count(*) FROM asset.devices WHERE status = 'OFFLINE'").Scan(&offline)
		db.QueryRow("SELECT count(*) FROM asset.monitors").Scan(&monitors)
		db.QueryRow("SELECT count(*) FROM asset.printers").Scan(&printers)
		db.QueryRow("SELECT count(*) FROM asset.software_catalog").Scan(&software)
		db.QueryRow(`
			SELECT count(*) FROM asset.devices 
			WHERE os_info->>'system' ILIKE '%Windows%' 
			   OR os_info->>'caption' ILIKE '%Windows%'
		`).Scan(&licenses)

		// 2. Computers by Status
		compByStatus := fiber.Map{
			"Ready (Online)":        online,
			"Unavailable (Offline)": offline,
		}

		// 3. Computers by Manufacturer, Computers by Type, and Storage Drive Types
		rows, err := db.Query(`
			SELECT d.hostname, d.bios_serial, h.motherboard_info, h.storage_info
			FROM asset.devices d
			LEFT JOIN asset.hardware_inventory h ON d.uuid = h.device_id
		`)
		if err != nil {
			return c.Status(500).SendString("Database error fetching devices hardware: " + err.Error())
		}
		defer rows.Close()

		mfgCounts := make(map[string]int)
		typeCounts := map[string]int{
			"Desktop PC":           0,
			"Laptop / Notebook":    0,
			"Virtual Machine (VM)": 0,
			"All-in-One (AIO)":     0,
		}
		storageCounts := map[string]int{
			"HDD Only":         0,
			"SSD Only":         0,
			"Both (SSD + HDD)": 0,
			"Unknown / None":   0,
		}

		for rows.Next() {
			var hostname, biosSerial string
			var motherboardBytes, storageBytes []byte
			rows.Scan(&hostname, &biosSerial, &motherboardBytes, &storageBytes)

			// Parse Motherboard Info
			var motherboard map[string]interface{}
			if len(motherboardBytes) > 0 {
				json.Unmarshal(motherboardBytes, &motherboard)
			}
			mfg := "Unknown"
			product := "Unknown"
			if motherboard != nil {
				if m, ok := motherboard["manufacturer"].(string); ok {
					mfg = m
				}
				if p, ok := motherboard["product"].(string); ok {
					product = p
				}
			}

			normalizedMfg := normalizeBrand(mfg)
			mfgCounts[normalizedMfg]++

			// Classify computer type
			deviceType := classifyDeviceType(mfg, product, hostname, biosSerial)
			typeCounts[deviceType]++

			// Analyze storage types
			var storage []map[string]interface{}
			if len(storageBytes) > 0 {
				json.Unmarshal(storageBytes, &storage)
			}

			hasSSD := false
			hasHDD := false
			if len(storage) > 0 {
				for _, disk := range storage {
					model := ""
					if m, ok := disk["model"].(string); ok {
						model = strings.ToLower(m)
					}
					media := ""
					if md, ok := disk["media_type"].(string); ok {
						media = strings.ToLower(md)
					}

					isSSD := strings.Contains(model, "ssd") ||
						strings.Contains(model, "nvme") ||
						strings.Contains(model, "solid state") ||
						strings.Contains(media, "ssd") ||
						strings.Contains(media, "solid state")

					isHDD := !isSSD && (strings.Contains(model, "hdd") ||
						strings.Contains(model, "hard disk") ||
						strings.Contains(model, "sata") ||
						strings.Contains(model, "scsi") ||
						model != "")

					if isSSD {
						hasSSD = true
					}
					if isHDD {
						hasHDD = true
					}
				}
			}

			if hasSSD && hasHDD {
				storageCounts["Both (SSD + HDD)"]++
			} else if hasSSD {
				storageCounts["SSD Only"]++
			} else if hasHDD {
				storageCounts["HDD Only"]++
			} else {
				storageCounts["Unknown / None"]++
			}
		}

		// Convert mfgCounts to list of {name, value}
		var computersByMfg []fiber.Map
		for name, val := range mfgCounts {
			computersByMfg = append(computersByMfg, fiber.Map{"name": name, "value": val})
		}

		// Convert typeCounts to list of {name, value}
		var computersByType []fiber.Map
		for name, val := range typeCounts {
			computersByType = append(computersByType, fiber.Map{"name": name, "value": val})
		}

		// 4. Monitors by Manufacturer
		monRows, err := db.Query(`
			SELECT COALESCE(manufacturer, 'Unknown') as mfg, COUNT(*) as qty
			FROM asset.monitors
			GROUP BY mfg
		`)
		var monitorsByMfg []fiber.Map
		if err == nil {
			defer monRows.Close()
			mfgMonCounts := make(map[string]int)
			for monRows.Next() {
				var mfg string
				var qty int
				monRows.Scan(&mfg, &qty)
				normalizedMon := normalizeBrand(mfg)
				mfgMonCounts[normalizedMon] += qty
			}
			for name, val := range mfgMonCounts {
				monitorsByMfg = append(monitorsByMfg, fiber.Map{"name": name, "value": val})
			}
		}

		// 5. Computers by Branch Location
		compBranchRows, err := db.Query(`
			SELECT COALESCE(b.name, 'Unassigned') as branch_name, COUNT(d.uuid) as qty
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.buildings b ON l.building_id = b.id
			GROUP BY branch_name
		`)
		var computersByBranch []fiber.Map
		if err == nil {
			defer compBranchRows.Close()
			for compBranchRows.Next() {
				var name string
				var val int
				compBranchRows.Scan(&name, &val)
				computersByBranch = append(computersByBranch, fiber.Map{"name": name, "value": val})
			}
		}

		// 6. Printers by Branch Location
		printBranchRows, err := db.Query(`
			SELECT COALESCE(b.name, 'Unassigned') as branch_name, COUNT(p.id) as qty
			FROM asset.printers p
			LEFT JOIN asset.departments dept ON p.department_id = dept.id
			LEFT JOIN asset.buildings b ON dept.building_id = b.id
			GROUP BY branch_name
		`)
		var printersByBranch []fiber.Map
		if err == nil {
			defer printBranchRows.Close()
			for printBranchRows.Next() {
				var name string
				var val int
				printBranchRows.Scan(&name, &val)
				printersByBranch = append(printersByBranch, fiber.Map{"name": name, "value": val})
			}
		}

		// 7. RAM quantity by size (bucketed)
		ramRows, err := db.Query(`
			SELECT 
			  CASE 
				WHEN total_ram <= 4.5 THEN '4 GB or less'
				WHEN total_ram > 4.5 AND total_ram <= 8.5 THEN '8 GB'
				WHEN total_ram > 8.5 AND total_ram <= 16.5 THEN '16 GB'
				WHEN total_ram > 16.5 AND total_ram <= 32.5 THEN '32 GB'
				ELSE '64 GB or more'
			  END as ram_bucket,
			  COUNT(*) as device_count
			FROM (
			  SELECT 
				device_id,
				COALESCE((
				  SELECT SUM((val->>'capacity_gb')::numeric)
				  FROM jsonb_array_elements(
					CASE WHEN jsonb_typeof(ram_info) = 'array' THEN ram_info ELSE '[]'::jsonb END
				  ) as val
				), 0) as total_ram
			  FROM asset.hardware_inventory
			) as ram_sums
			GROUP BY ram_bucket
		`)

		ramMap := map[string]int{
			"4 GB or less":  0,
			"8 GB":          0,
			"16 GB":         0,
			"32 GB":         0,
			"64 GB or more": 0,
		}
		if err == nil {
			defer ramRows.Close()
			for ramRows.Next() {
				var bucket string
				var count int
				ramRows.Scan(&bucket, &count)
				ramMap[bucket] = count
			}
		}

		ramBySize := []fiber.Map{
			{"name": "4 GB or less", "value": ramMap["4 GB or less"]},
			{"name": "8 GB", "value": ramMap["8 GB"]},
			{"name": "16 GB", "value": ramMap["16 GB"]},
			{"name": "32 GB", "value": ramMap["32 GB"]},
			{"name": "64 GB or more", "value": ramMap["64 GB or more"]},
		}

		// 8. Network Connections (LAN / Wi-Fi)
		netRows, err := db.Query(`
			SELECT COALESCE(os_info->>'connection_type', 'LAN') as conn_type, COUNT(*) as qty
			FROM asset.devices
			GROUP BY conn_type
		`)
		netMap := map[string]int{
			"LAN":   0,
			"Wi-Fi": 0,
		}
		if err == nil {
			defer netRows.Close()
			for netRows.Next() {
				var ctype string
				var qty int
				netRows.Scan(&ctype, &qty)
				if ctype == "Wi-Fi" {
					netMap["Wi-Fi"] = qty
				} else {
					netMap["LAN"] += qty
				}
			}
		}

		// 9. Overtime (online >2 days)
		var overtimeCount int
		var overtimeDept string
		err = db.QueryRow(`
			WITH active_devices AS (
				SELECT uuid, location_id FROM asset.devices WHERE status = 'ONLINE'
			),
			uptime_span AS (
				SELECT device_id
				FROM asset.heartbeats
				WHERE captured_at >= NOW() - INTERVAL '3 days'
				GROUP BY device_id
				HAVING MAX(captured_at) - MIN(captured_at) >= INTERVAL '2 days'
				   AND MAX(captured_at) >= NOW() - INTERVAL '15 minutes'
			)
			SELECT COUNT(*), COALESCE(dept.name, 'Unassigned') as dept_name
			FROM uptime_span u
			JOIN active_devices d ON u.device_id = d.uuid
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			GROUP BY dept_name
			ORDER BY COUNT(*) DESC
			LIMIT 1
		`).Scan(&overtimeCount, &overtimeDept)
		if err != nil {
			overtimeCount = 0
			overtimeDept = "None"
		}

		return c.JSON(fiber.Map{
			"kpis": fiber.Map{
				"computers": computers,
				"online":    online,
				"offline":   offline,
				"monitors":  monitors,
				"printers":  printers,
				"software":  software,
				"licenses":  licenses,
			},
			"computers_by_status":       compByStatus,
			"computers_by_manufacturer": computersByMfg,
			"computers_by_type":         computersByType,
			"monitors_by_manufacturer":  monitorsByMfg,
			"computers_by_branch":       computersByBranch,
			"printers_by_branch":        printersByBranch,
			"ram_by_size":               ramBySize,
			"storage_types":             storageCounts,
			"network_connections":       netMap,
			"overtime_count":            overtimeCount,
			"overtime_dept":             overtimeDept,
		})
	})

	// GET /devices — Dashboard device list
	v1.Get("/devices", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT 
				d.uuid, d.hostname, d.ip_address, d.mac_address, d.os_info, d.windows_build,
				d.bios_serial, d.last_user, d.status, d.last_seen, d.device_type, COALESCE(d.asset_number, '') as asset_number, d.allow_overtime,
				COALESCE(b.name, '') as building_name,
				COALESCE(dept.name, '') as department_name,
				COALESCE(df.facts->'defender'->>'antivirus_enabled', 'false') as av_enabled,
				COALESCE(df.facts->'defender'->>'antivirus_signature_version', df.facts->'defender'->>'engine_version', '') as av_version,
				COALESCE(sec.product_name, '') as security_product_name,
				COALESCE(sec.vendor, '') as security_vendor,
				COALESCE(sec.realtime_protection_enabled, NULLIF(df.facts->'defender'->>'realtime_protection_enabled', '')::boolean, false) as security_protected,
				COALESCE(sec.signature_status, '') as security_signature_status,
				COALESCE(h.cpu_info->>'name', '') as cpu_name,
				COALESCE((SELECT SUM((val->>'capacity_gb')::numeric) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(h.ram_info) = 'array' THEN h.ram_info ELSE '[]'::jsonb END) as val), 0) as ram_gb,
				COALESCE((SELECT SUM((val->>'size_gb')::numeric) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(h.storage_info) = 'array' THEN h.storage_info ELSE '[]'::jsonb END) as val), 0) as storage_gb,
				(SELECT COUNT(*) FROM asset.software_installations si WHERE si.device_id = d.uuid) as software_count,
				EXISTS(SELECT 1 FROM asset.software_installations si JOIN asset.software_catalog sc ON si.software_id = sc.id WHERE si.device_id = d.uuid AND sc.software_name ILIKE '%HOSxP%') as has_hosxp,
				COALESCE((SELECT json_agg(json_build_object('name', p.printer_name, 'brand', p.brand, 'type', p.printer_type, 'last_seen', p.last_seen_at)) FROM asset.printers p WHERE p.device_id = d.uuid), '[]'::json) as connected_printers,
				COALESCE((SELECT string_agg(size_inch::text || ' inc', ', ') FROM asset.monitors m WHERE m.device_id = d.uuid), '') as screen_sizes,
				COALESCE((SELECT bool_or((val->>'predict_failure')::boolean) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(h.storage_info) = 'array' THEN h.storage_info ELSE '[]'::jsonb END) as val), false) as disk_failed
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.buildings b ON l.building_id = b.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			LEFT JOIN asset.device_facts df ON d.uuid = df.device_id
			LEFT JOIN asset.hardware_inventory h ON d.uuid = h.device_id
			LEFT JOIN LATERAL (
				SELECT product_name, vendor, realtime_protection_enabled, signature_status
				FROM asset.security_products sp WHERE sp.device_id = d.uuid
				ORDER BY sp.realtime_protection_enabled DESC, sp.detected_at DESC LIMIT 1
			) sec ON TRUE
			ORDER BY d.last_seen DESC
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var result []fiber.Map
		for rows.Next() {
			var uuid, hostname, ip, mac, winBuild, biosSerial, lastUser, status, devType, assetNumber, bName, deptName, avEnabled, avVersion, securityName, securityVendor, securitySignatureStatus string
			var cpuName, screenSizes string
			var connectedPrintersJSON []byte
			var ramGb, storageGb float64
			var softwareCount int
			var hasHosxp, allowOvertime, diskFailed, securityProtected bool
			var osInfo []byte
			var lastSeen *time.Time

			err = rows.Scan(
				&uuid, &hostname, &ip, &mac, &osInfo, &winBuild, &biosSerial, &lastUser, &status, &lastSeen, &devType, &assetNumber, &allowOvertime, &bName, &deptName, &avEnabled, &avVersion, &securityName, &securityVendor, &securityProtected, &securitySignatureStatus,
				&cpuName, &ramGb, &storageGb, &softwareCount, &hasHosxp, &connectedPrintersJSON, &screenSizes, &diskFailed,
			)
			if err != nil {
				log.Printf("[DevicesList] Scan error: %v", err)
				continue
			}

			var osMap map[string]interface{}
			json.Unmarshal(osInfo, &osMap)

			lastSeenStr := ""
			if lastSeen != nil {
				lastSeenStr = lastSeen.Format(time.RFC3339)
			}

			var printersList []map[string]interface{}
			if len(connectedPrintersJSON) > 0 {
				_ = json.Unmarshal(connectedPrintersJSON, &printersList)
			}
			if printersList == nil {
				printersList = []map[string]interface{}{}
			}

			result = append(result, fiber.Map{
				"uuid":                      uuid,
				"hostname":                  hostname,
				"ip_address":                ip,
				"mac_address":               mac,
				"os_info":                   osMap,
				"windows_build":             winBuild,
				"bios_serial":               biosSerial,
				"last_user":                 lastUser,
				"status":                    status,
				"last_seen":                 lastSeenStr,
				"device_type":               devType,
				"asset_number":              assetNumber,
				"building_name":             bName,
				"department_name":           deptName,
				"av_enabled":                avEnabled,
				"av_version":                avVersion,
				"security_product_name":     securityName,
				"security_vendor":           securityVendor,
				"security_protected":        securityProtected,
				"security_signature_status": securitySignatureStatus,
				"cpu_name":                  cpuName,
				"ram_gb":                    ramGb,
				"storage_gb":                storageGb,
				"software_count":            softwareCount,
				"has_hosxp":                 hasHosxp,
				"allow_overtime":            allowOvertime,
				"disk_failed":               diskFailed,
				"connected_printers":        printersList,
				"screen_sizes":              screenSizes,
			})
		}

		if result == nil {
			result = []fiber.Map{}
		}
		return c.JSON(result)
	})

	// GET /agent/tasks — Agent polls for pending tasks
	v1.Get("/agent/tasks", verifyAgent, func(c *fiber.Ctx) error {
		uuid := c.Query("uuid")
		if uuid == "" {
			return c.Status(400).JSON(fiber.Map{"error": "uuid required"})
		}

		rows, err := db.Query(`
			SELECT id, custom_script
			FROM asset.task_queue
			WHERE device_id = $1 AND status = 'pending'
			ORDER BY created_at ASC
			LIMIT 5
		`, uuid)
		if err != nil {
			return c.JSON([]fiber.Map{})
		}
		defer rows.Close()

		var tasks []fiber.Map
		for rows.Next() {
			var id int
			var script string
			rows.Scan(&id, &script)

			tasks = append(tasks, fiber.Map{
				"id":           id,
				"command_type": script,
			})
		}

		if tasks == nil {
			tasks = []fiber.Map{}
		}
		return c.JSON(tasks)
	})

	// POST /agent/screenshot — Agent uploads captured image
	v1.Post("/agent/screenshot", verifyAgent, func(c *fiber.Ctx) error {
		uuid := c.Query("uuid")
		taskID := c.Query("task_id")
		imgData := string(c.Body())

		if uuid == "" || imgData == "" {
			return c.Status(400).SendString("uuid and image data required")
		}

		_, err := db.Exec(`
			INSERT INTO asset.screenshots (device_id, task_id, image_data)
			VALUES ($1, $2, $3)
		`, uuid, taskID, imgData)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.SendString("Screenshot saved")
	})

	// POST /agent/tasks/:id/result — Agent reports task completion
	v1.Post("/agent/tasks/:id/result", verifyAgent, func(c *fiber.Ctx) error {
		id := c.Params("id")
		var req struct {
			Status string `json:"status"`
			Output string `json:"output"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Bad Request")
		}

		_, err := db.Exec(`
			UPDATE asset.task_queue 
			SET status = $1, result_output = $2, completed_at = CURRENT_TIMESTAMP
			WHERE id = $3
		`, req.Status, req.Output, id)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.SendString("Result updated")
	})

	// GET /update/check — Agent checks for system updates
	v1.Get("/update/check", verifyAgent, func(c *fiber.Ctx) error {
		baseDir, _ := os.Getwd()
		var path string
		if _, err := os.Stat(filepath.Join(baseDir, "latest_version.json")); err == nil {
			path = filepath.Join(baseDir, "latest_version.json")
		} else if _, err := os.Stat(filepath.Join(baseDir, "server", "latest_version.json")); err == nil {
			path = filepath.Join(baseDir, "server", "latest_version.json")
		} else {
			path = "latest_version.json"
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return c.Status(500).SendString("Could not read update config: " + err.Error())
		}

		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			return c.Status(500).SendString("Error parsing config: " + err.Error())
		}
		return c.JSON(config)
	})

	// GET /update/download — Agent downloads updated agent.py script
	v1.Get("/update/download", verifyAgent, func(c *fiber.Ctx) error {
		baseDir, _ := os.Getwd()
		var path string

		if _, err := os.Stat(filepath.Join(baseDir, "agent_new.py")); err == nil {
			path = filepath.Join(baseDir, "agent_new.py")
		} else if _, err := os.Stat(filepath.Join(baseDir, "server", "agent_new.py")); err == nil {
			path = filepath.Join(baseDir, "server", "agent_new.py")
		} else if _, err := os.Stat(filepath.Join(baseDir, "agent.py")); err == nil {
			path = filepath.Join(baseDir, "agent.py")
		} else if _, err := os.Stat(filepath.Join(baseDir, "server", "agent.py")); err == nil {
			path = filepath.Join(baseDir, "server", "agent.py")
		} else if _, err := os.Stat(filepath.Join(filepath.Dir(baseDir), "agent", "agent.py")); err == nil {
			path = filepath.Join(filepath.Dir(baseDir), "agent", "agent.py")
		} else if _, err := os.Stat(filepath.Join(baseDir, "agent", "agent.py")); err == nil {
			path = filepath.Join(baseDir, "agent", "agent.py")
		} else {
			return c.Status(404).SendString("Update file not found")
		}

		return c.Download(path, "agent.py")
	})

	// GET /devices/:uuid — Device Detail (hardware + software + heartbeat)
	v1.Get("/devices/:uuid", verifyAdmin, func(c *fiber.Ctx) error {
		uuid := c.Params("uuid")

		var hostname, ip, mac, winBuild, biosSerial, lastUser, status, devType, assetNumber string
		var locationID *int
		var allowOvertime bool
		var bName, deptName string
		var osInfo []byte
		var lastSeen *time.Time
		err := db.QueryRow(`
			SELECT 
				d.uuid, d.hostname, d.ip_address, d.mac_address, d.os_info, d.windows_build,
				d.bios_serial, d.last_user, d.status, d.last_seen, d.device_type, 
				COALESCE(d.asset_number, '') as asset_number, d.location_id, d.allow_overtime,
				COALESCE(b.name, '') as building_name,
				COALESCE(dept.name, '') as department_name
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.buildings b ON l.building_id = b.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			WHERE d.uuid = $1`, uuid).Scan(
			&uuid, &hostname, &ip, &mac, &osInfo, &winBuild,
			&biosSerial, &lastUser, &status, &lastSeen, &devType, &assetNumber, &locationID, &allowOvertime,
			&bName, &deptName)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Device not found"})
		}

		var osMap map[string]interface{}
		json.Unmarshal(osInfo, &osMap)

		lastSeenStr := ""
		if lastSeen != nil {
			lastSeenStr = lastSeen.Format(time.RFC3339)
		}

		var cpuInfo, ramInfo, storageInfo, gpuInfo, mbInfo, biosInfo []byte
		db.QueryRow(`
			SELECT cpu_info, ram_info, storage_info, gpu_info, motherboard_info, bios_info
			FROM asset.hardware_inventory WHERE device_id = $1`, uuid).Scan(
			&cpuInfo, &ramInfo, &storageInfo, &gpuInfo, &mbInfo, &biosInfo)

		var cpu, ram, storage, gpu, mb, bios interface{}
		json.Unmarshal(cpuInfo, &cpu)
		json.Unmarshal(ramInfo, &ram)
		json.Unmarshal(storageInfo, &storage)
		json.Unmarshal(gpuInfo, &gpu)
		json.Unmarshal(mbInfo, &mb)
		json.Unmarshal(biosInfo, &bios)

		swRows, _ := db.Query(`
			SELECT sc.software_name, si.version, si.install_path, si.detected_at,
			       EXISTS(SELECT 1 FROM asset.software_blacklist b WHERE b.software_name = sc.software_name) as is_blacklisted
			FROM asset.software_installations si
			JOIN asset.software_catalog sc ON sc.id = si.software_id
			WHERE si.device_id = $1
			ORDER BY sc.software_name`, uuid)
		var software []fiber.Map
		if swRows != nil {
			defer swRows.Close()
			for swRows.Next() {
				var name, version, path string
				var detectedAt *time.Time
				var isBL bool
				swRows.Scan(&name, &version, &path, &detectedAt, &isBL)
				detStr := ""
				if detectedAt != nil {
					detStr = detectedAt.Format(time.RFC3339)
				}
				software = append(software, fiber.Map{
					"name": name, "version": version,
					"install_path": path, "detected_at": detStr,
					"is_blacklisted": isBL,
				})
			}
		}

		usbRows, _ := db.Query("SELECT id, device_name, device_type, detected_at FROM asset.usb_devices WHERE device_id = $1 ORDER BY detected_at DESC", uuid)
		var usbs []fiber.Map
		if usbRows != nil {
			defer usbRows.Close()
			for usbRows.Next() {
				var id int
				var name, dtype, cat string
				usbRows.Scan(&id, &name, &dtype, &cat)
				usbs = append(usbs, fiber.Map{"id": id, "name": name, "type": dtype, "date": cat})
			}
		}

		logRows, _ := db.Query("SELECT id, change_type, field_name, old_value, new_value, created_at FROM asset.change_logs WHERE device_id = $1 AND change_type <> 'STATUS' ORDER BY created_at DESC LIMIT 50", uuid)
		var logs []fiber.Map
		if logRows != nil {
			defer logRows.Close()
			for logRows.Next() {
				var id int
				var ctype, field, oldv, newv, cat string
				logRows.Scan(&id, &ctype, &field, &oldv, &newv, &cat)
				logs = append(logs, fiber.Map{"id": id, "type": ctype, "field": field, "old": oldv, "new": newv, "date": cat})
			}
		}

		var cpuPct, ramPct, diskPct float64
		db.QueryRow(`
			SELECT cpu_pct, ram_pct, disk_pct FROM asset.heartbeats
			WHERE device_id = $1 ORDER BY created_at DESC LIMIT 1`, uuid).Scan(&cpuPct, &ramPct, &diskPct)

		// Extra agent facts (device_facts)
		var factsBytes []byte
		var factsUpdatedAt *time.Time
		_ = db.QueryRow(`
			SELECT facts, updated_at
			FROM asset.device_facts
			WHERE device_id = $1
		`, uuid).Scan(&factsBytes, &factsUpdatedAt)

		var factsMap map[string]interface{}
		if len(factsBytes) > 0 {
			_ = json.Unmarshal(factsBytes, &factsMap)
		}
		factsUpdatedAtStr := ""
		if factsUpdatedAt != nil {
			factsUpdatedAtStr = factsUpdatedAt.Format(time.RFC3339)
		}

		securityProducts := []fiber.Map{}
		securityRows, err := db.Query(`
			SELECT product_name, vendor, realtime_protection_enabled, signature_status, source, raw_data, detected_at
			FROM asset.security_products WHERE device_id = $1
			ORDER BY realtime_protection_enabled DESC, product_name`, uuid)
		if err == nil {
			defer securityRows.Close()
			for securityRows.Next() {
				var name, vendor, signatureStatus, source string
				var realtime bool
				var rawData []byte
				var detectedAt time.Time
				if err := securityRows.Scan(&name, &vendor, &realtime, &signatureStatus, &source, &rawData, &detectedAt); err != nil {
					continue
				}
				var raw map[string]interface{}
				_ = json.Unmarshal(rawData, &raw)
				securityProducts = append(securityProducts, fiber.Map{
					"name": name, "vendor": vendor, "realtime_protection_enabled": realtime,
					"signature_status": signatureStatus, "source": source,
					"detected_at": detectedAt.Format(time.RFC3339), "details": raw,
				})
			}
		}

		// Master Data (device_assets)
		var sku, typeICT, comment string
		var amount float64
		var purchaseDate, warrantyExpire *time.Time
		var warrantyMonths int

		_ = db.QueryRow(`
			SELECT sku, type_ict, purchase_date, warranty_months, warranty_expire, amount, comment
			FROM asset.device_assets WHERE device_id = $1`, uuid).Scan(
			&sku, &typeICT, &purchaseDate, &warrantyMonths, &warrantyExpire, &amount, &comment)

		purchaseDateStr, warrantyExpireStr := "", ""
		if purchaseDate != nil {
			purchaseDateStr = purchaseDate.Format("2006-01-02")
		}
		if warrantyExpire != nil {
			warrantyExpireStr = warrantyExpire.Format("2006-01-02")
		}

		// Query USB-only printers for this device (USB physically connected)
		pRows, pErr := db.Query(`
			SELECT id, printer_name, brand, model, ip, printer_type, total_pages_printed, COALESCE(last_ink_change::text, '')
			FROM asset.printers WHERE device_id = $1 AND printer_type = 'USB' ORDER BY printer_name
		`, uuid)
		var devicePrinters []fiber.Map
		if pErr == nil {
			defer pRows.Close()
			for pRows.Next() {
				var pid, totalPages int
				var pName, pBrand, pModel, pIp, pType, pInk string
				pRows.Scan(&pid, &pName, &pBrand, &pModel, &pIp, &pType, &totalPages, &pInk)
				devicePrinters = append(devicePrinters, fiber.Map{
					"id": pid, "printer_name": pName, "name": pName,
					"brand": pBrand, "model": pModel, "ip": pIp, "type": pType,
					"total_pages_printed": totalPages, "last_ink": pInk,
				})
			}
		}

		return c.JSON(fiber.Map{
			"uuid": uuid, "hostname": hostname, "ip_address": ip,
			"mac_address": mac, "os_info": osMap, "windows_build": winBuild,
			"bios_serial": biosSerial, "last_user": lastUser,
			"status": status, "last_seen": lastSeenStr, "device_type": devType,
			"asset_number": assetNumber, "location_id": locationID,
			"allow_overtime": allowOvertime,
			"building_name":  bName, "department_name": deptName,
			"hardware": fiber.Map{
				"cpu": cpu, "ram": ram, "storage": storage,
				"gpu": gpu, "motherboard": mb, "bios": bios,
			},
			"printers":          devicePrinters,
			"software":          software,
			"usb":               usbs,
			"logs":              logs,
			"stats":             fiber.Map{"cpu_pct": cpuPct, "ram_pct": ramPct, "disk_pct": diskPct},
			"facts":             factsMap,
			"facts_updated_at":  factsUpdatedAtStr,
			"security_products": securityProducts,
			"master_data": fiber.Map{
				"sku": sku, "type_ict": typeICT, "purchase_date": purchaseDateStr,
				"warranty_months": warrantyMonths, "warranty_expire": warrantyExpireStr,
				"amount": amount, "comment": comment,
			},
		})
	})

	// PUT /devices/:uuid/asset — Update Master Data (Manual Update)
	v1.Put("/devices/:uuid/asset", verifyAdmin, func(c *fiber.Ctx) error {
		uuid := c.Params("uuid")
		var req struct {
			SKU            string  `json:"sku"`
			TypeICT        string  `json:"type_ict"`
			PurchaseDate   string  `json:"purchase_date"`
			WarrantyMonths int     `json:"warranty_months"`
			WarrantyExpire string  `json:"warranty_expire"`
			Amount         float64 `json:"amount"`
			Comment        string  `json:"comment"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Bad Request")
		}

		var pDate, wExpire interface{}
		if req.PurchaseDate != "" {
			pDate = req.PurchaseDate
		}
		if req.WarrantyExpire != "" {
			wExpire = req.WarrantyExpire
		}

		_, err := db.Exec(`
			INSERT INTO asset.device_assets (device_id, sku, type_ict, purchase_date, warranty_months, warranty_expire, amount, comment, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP)
			ON CONFLICT (device_id) DO UPDATE SET
				sku = EXCLUDED.sku,
				type_ict = EXCLUDED.type_ict,
				purchase_date = EXCLUDED.purchase_date,
				warranty_months = EXCLUDED.warranty_months,
				warranty_expire = EXCLUDED.warranty_expire,
				amount = EXCLUDED.amount,
				comment = EXCLUDED.comment,
				updated_at = CURRENT_TIMESTAMP
		`, uuid, req.SKU, req.TypeICT, pDate, req.WarrantyMonths, wExpire, req.Amount, req.Comment)

		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.SendString("Asset data updated")
	})

	// PUT /devices/:uuid — Update computer metadata (asset_number, location_id, allow_overtime)
	v1.Put("/devices/:uuid", verifyAdmin, func(c *fiber.Ctx) error {
		uuid := c.Params("uuid")
		var req struct {
			AssetNumber   string `json:"asset_number"`
			LocationID    *int   `json:"location_id"`
			AllowOvertime *bool  `json:"allow_overtime"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Bad Request"})
		}

		var oldAssetNumber string
		var oldLocationID *int
		var oldAllowOvertime bool
		_ = db.QueryRow("SELECT COALESCE(asset_number, ''), location_id, allow_overtime FROM asset.devices WHERE uuid = $1", uuid).Scan(&oldAssetNumber, &oldLocationID, &oldAllowOvertime)

		allowOvertimeVal := oldAllowOvertime
		if req.AllowOvertime != nil {
			allowOvertimeVal = *req.AllowOvertime
		}

		_, err := db.Exec(`
			UPDATE asset.devices
			SET asset_number = $1, location_id = $2, allow_overtime = $3
			WHERE uuid = $4`, req.AssetNumber, req.LocationID, allowOvertimeVal, uuid)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		if oldAssetNumber != req.AssetNumber {
			logChange(uuid, "METADATA", "Asset Number", oldAssetNumber, req.AssetNumber)
		}

		oldLocStr := "None"
		if oldLocationID != nil {
			oldLocStr = fmt.Sprintf("ID:%d", *oldLocationID)
		}
		newLocStr := "None"
		if req.LocationID != nil {
			newLocStr = fmt.Sprintf("ID:%d", *req.LocationID)
		}
		if oldLocStr != newLocStr {
			logChange(uuid, "METADATA", "Location ID", oldLocStr, newLocStr)
		}

		if oldAllowOvertime != allowOvertimeVal {
			logChange(uuid, "METADATA", "Allow Overtime", fmt.Sprintf("%v", oldAllowOvertime), fmt.Sprintf("%v", allowOvertimeVal))
		}

		return c.JSON(fiber.Map{"status": "ok", "message": "Metadata updated successfully"})
	})

	// PUT /devices/bulk-location — Bulk update devices location/department
	v1.Put("/devices/bulk-location", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			DeviceUUIDs []string `json:"device_uuids"`
			LocationID  *int     `json:"location_id"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Bad Request"})
		}
		if len(req.DeviceUUIDs) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "No devices selected"})
		}

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer tx.Rollback()

		for _, uuid := range req.DeviceUUIDs {
			var oldLocationID *int
			_ = tx.QueryRow("SELECT location_id FROM asset.devices WHERE uuid = $1", uuid).Scan(&oldLocationID)

			_, err = tx.Exec(`
				UPDATE asset.devices
				SET location_id = $1
				WHERE uuid = $2`, req.LocationID, uuid)
			if err != nil {
				return c.Status(500).SendString(err.Error())
			}

			oldLocStr := "None"
			if oldLocationID != nil {
				oldLocStr = fmt.Sprintf("ID:%d", *oldLocationID)
			}
			newLocStr := "None"
			if req.LocationID != nil {
				newLocStr = fmt.Sprintf("ID:%d", *req.LocationID)
			}
			if oldLocStr != newLocStr {
				_, _ = tx.Exec(`INSERT INTO asset.change_logs (device_id, change_type, field_name, old_value, new_value)
				                VALUES ($1, 'METADATA', 'Location ID', $2, $3)`, uuid, oldLocStr, newLocStr)
			}
		}

		if err := tx.Commit(); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		return c.JSON(fiber.Map{"status": "ok", "message": fmt.Sprintf("Successfully updated %d device(s)", len(req.DeviceUUIDs))})
	})

	// DELETE /devices/:uuid — Delete computer
	v1.Delete("/devices/:uuid", verifyAdmin, func(c *fiber.Ctx) error {
		uuid := c.Params("uuid")
		_, err := db.Exec("DELETE FROM asset.devices WHERE uuid = $1", uuid)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		fmt.Printf("[*] Device deleted: %s\n", uuid)
		return c.SendString("Deleted")
	})

	// GET /software — Global software registry
	v1.Get("/software", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT sc.id, sc.software_name,
			       COUNT(DISTINCT si.device_id) as install_count,
			       array_agg(DISTINCT si.version) as versions,
			       EXISTS(SELECT 1 FROM asset.software_blacklist b WHERE b.software_name = sc.software_name) as is_blacklisted
			FROM asset.software_catalog sc
			LEFT JOIN asset.software_installations si ON si.software_id = sc.id
			GROUP BY sc.id, sc.software_name
			ORDER BY install_count DESC, sc.software_name
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var result []fiber.Map
		for rows.Next() {
			var id, count int
			var name string
			var versionsRaw []byte
			var isBL bool
			rows.Scan(&id, &name, &count, &versionsRaw, &isBL)
			result = append(result, fiber.Map{
				"id": id, "name": name,
				"install_count":  count,
				"versions_raw":   string(versionsRaw),
				"is_blacklisted": isBL,
			})
		}
		if result == nil {
			result = []fiber.Map{}
		}
		return c.JSON(result)
	})

	// POST /tasks — Dashboard creates task for device (With resolved templates!)
	v1.Post("/tasks", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			DeviceUUID  string                 `json:"device_uuid"`
			CommandType string                 `json:"command_type"`
			Payload     map[string]interface{} `json:"payload"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Bad Request"})
		}
		if req.DeviceUUID == "" || req.CommandType == "" {
			return c.Status(400).JSON(fiber.Map{"error": "device_uuid and command_type required"})
		}

		var templateID sql.NullInt64
		var scriptContent string

		err := db.QueryRow(`
			SELECT id, script_content 
			FROM asset.command_templates 
			WHERE cmd_name = $1
		`, req.CommandType).Scan(&templateID, &scriptContent)

		if err == sql.ErrNoRows {
			// If not found in templates, treat it as a raw custom script
			scriptContent = req.CommandType
			templateID = sql.NullInt64{Valid: false}
		} else if err != nil {
			return c.Status(500).SendString("Database error checking templates: " + err.Error())
		}

		var taskID int
		err = db.QueryRow(`
			INSERT INTO asset.task_queue (device_id, cmd_template_id, status, custom_script, created_by)
			VALUES ($1, $2, 'pending', $3, 'admin') RETURNING id`,
			req.DeviceUUID, templateID, scriptContent).Scan(&taskID)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(fiber.Map{"status": "queued", "task_id": taskID})
	})

	// GET /tasks/recent — Dashboard fetches active/recent task logs
	v1.Get("/tasks/recent", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT t.id, t.device_id, COALESCE(d.hostname, 'Unknown') as hostname,
			       COALESCE(t.custom_script, '') as script,
			       t.status, COALESCE(t.result_output, '') as output,
			       t.created_at, COALESCE(t.completed_at::text, '') as completed_at
			FROM asset.task_queue t
			LEFT JOIN asset.devices d ON t.device_id = d.uuid
			ORDER BY t.id DESC LIMIT 30
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var result []fiber.Map
		for rows.Next() {
			var id int
			var devID, host, script, st, out, cat, compAt string
			rows.Scan(&id, &devID, &host, &script, &st, &out, &cat, &compAt)
			result = append(result, fiber.Map{
				"id":           id,
				"device_id":    devID,
				"hostname":     host,
				"script":       script,
				"status":       st,
				"output":       out,
				"created_at":   cat,
				"completed_at": compAt,
			})
		}
		if result == nil {
			result = []fiber.Map{}
		}
		return c.JSON(result)
	})

	// GET /screenshots — Gallery of captured screens
	v1.Get("/screenshots", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT s.id, s.device_id, COALESCE(d.hostname, s.device_id, 'Unknown') as hostname, s.created_at 
			FROM asset.screenshots s
			LEFT JOIN asset.devices d ON LOWER(s.device_id) = LOWER(d.uuid)
			ORDER BY s.created_at DESC LIMIT 50
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		var result []fiber.Map
		for rows.Next() {
			var id int
			var devID, hostname, createdAt string
			rows.Scan(&id, &devID, &hostname, &createdAt)
			result = append(result, fiber.Map{
				"id": id, "device_id": devID, "hostname": hostname, "created_at": createdAt,
			})
		}
		if result == nil {
			result = []fiber.Map{}
		}
		return c.JSON(result)
	})

	// GET /screenshots/:id — Get base64 image data
	v1.Get("/screenshots/:id", verifyAdmin, func(c *fiber.Ctx) error {
		id := c.Params("id")
		var imgData string
		err := db.QueryRow("SELECT image_data FROM asset.screenshots WHERE id = $1", id).Scan(&imgData)
		if err != nil {
			return c.Status(404).SendString("Not found")
		}

		imgData = strings.TrimSpace(imgData)
		if idx := strings.Index(imgData, ","); idx != -1 && strings.Contains(imgData[:idx], "base64") {
			imgData = imgData[idx+1:]
		}

		imgBytes, err := base64.StdEncoding.DecodeString(imgData)
		if err != nil {
			imgBytes, err = base64.RawStdEncoding.DecodeString(imgData)
		}
		if err != nil {
			return c.Status(500).SendString("Error decoding image: " + err.Error())
		}

		c.Set("Content-Type", "image/jpeg")
		return c.Send(imgBytes)
	})

	// DELETE /screenshots/:id — Delete single screenshot
	v1.Delete("/screenshots/:id", verifyAdmin, func(c *fiber.Ctx) error {
		id := c.Params("id")
		res, err := db.Exec("DELETE FROM asset.screenshots WHERE id = $1", id)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return c.Status(404).SendString("Screenshot not found")
		}
		return c.SendString("OK")
	})

	// GET /logs — Admin audit logs
	v1.Get("/logs", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT l.id, l.device_id, d.hostname, l.change_type, l.field_name, l.old_value, l.new_value, l.created_at
			FROM asset.change_logs l
			JOIN asset.devices d ON l.device_id = d.uuid
			WHERE l.change_type <> 'STATUS'
			ORDER BY l.created_at DESC LIMIT 100
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		var logs []fiber.Map
		for rows.Next() {
			var id int
			var devID, host, ctype, field, oldv, newv, cat string
			rows.Scan(&id, &devID, &host, &ctype, &field, &oldv, &newv, &cat)
			logs = append(logs, fiber.Map{
				"id": id, "hostname": host, "type": ctype, "field": field, "old": oldv, "new": newv, "date": cat,
			})
		}
		return c.JSON(logs)
	})

	// Blacklist
	v1.Get("/blacklist", verifyAdmin, func(c *fiber.Ctx) error {
		rows, _ := db.Query("SELECT id, software_name, reason, created_at FROM asset.software_blacklist ORDER BY created_at DESC")
		var list []fiber.Map
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var id int
				var name, reason, cat string
				rows.Scan(&id, &name, &reason, &cat)
				list = append(list, fiber.Map{"id": id, "name": name, "reason": reason, "date": cat})
			}
		}
		return c.JSON(list)
	})

	v1.Post("/blacklist", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil || req.Name == "" {
			return c.Status(400).SendString("Invalid data")
		}
		db.Exec("INSERT INTO asset.software_blacklist (software_name, reason) VALUES ($1, $2) ON CONFLICT DO NOTHING", req.Name, req.Reason)
		return c.SendString("Added")
	})

	v1.Delete("/blacklist/:id", verifyAdmin, func(c *fiber.Ctx) error {
		db.Exec("DELETE FROM asset.software_blacklist WHERE id = $1", c.Params("id"))
		return c.SendString("Deleted")
	})

	// Master Data (Buildings/Depts)
	v1.Get("/master/buildings", verifyAdmin, func(c *fiber.Ctx) error {
		rows, _ := db.Query("SELECT id, name, code FROM asset.buildings ORDER BY name")
		var res []fiber.Map
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var id int
				var name, code string
				rows.Scan(&id, &name, &code)
				res = append(res, fiber.Map{"id": id, "name": name, "code": code})
			}
		}
		return c.JSON(res)
	})

	v1.Post("/master/buildings", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			Name string `json:"name"`
			Code string `json:"code"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			return c.Status(400).SendString("Building name is required")
		}
		if _, err := db.Exec("INSERT INTO asset.buildings (name, code) VALUES ($1, $2)", strings.TrimSpace(req.Name), strings.TrimSpace(req.Code)); err != nil {
			return c.Status(400).SendString(err.Error())
		}
		return c.SendString("OK")
	})

	v1.Delete("/master/buildings/:id", verifyAdmin, func(c *fiber.Ctx) error {
		result, err := db.Exec("DELETE FROM asset.buildings WHERE id = $1", c.Params("id"))
		if err != nil {
			return c.Status(400).SendString(err.Error())
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return c.Status(404).SendString("Building not found")
		}
		return c.SendString("Deleted")
	})

	// Departments are reusable master records. They may optionally be linked to one building.
	v1.Get("/master/departments", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`SELECT d.id, d.name, d.code, d.building_id, COALESCE(b.name, '')
			FROM asset.departments d LEFT JOIN asset.buildings b ON b.id = d.building_id ORDER BY b.name, d.name`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		res := []fiber.Map{}
		for rows.Next() {
			var id int
			var name, code, buildingName string
			var buildingID sql.NullInt64
			if err := rows.Scan(&id, &name, &code, &buildingID, &buildingName); err != nil {
				return c.Status(500).SendString(err.Error())
			}
			var building interface{}
			if buildingID.Valid {
				building = buildingID.Int64
			}
			res = append(res, fiber.Map{"id": id, "name": name, "code": code, "building_id": building, "building_name": buildingName})
		}
		return c.JSON(res)
	})

	v1.Post("/master/departments", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			Name       string `json:"name"`
			Code       string `json:"code"`
			BuildingID *int   `json:"building_id"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			return c.Status(400).SendString("Department name is required")
		}
		if _, err := db.Exec("INSERT INTO asset.departments (name, code, building_id) VALUES ($1, $2, $3)", strings.TrimSpace(req.Name), strings.TrimSpace(req.Code), req.BuildingID); err != nil {
			return c.Status(400).SendString(err.Error())
		}
		return c.SendString("OK")
	})

	v1.Delete("/master/departments/:id", verifyAdmin, func(c *fiber.Ctx) error {
		result, err := db.Exec("DELETE FROM asset.departments WHERE id = $1", c.Params("id"))
		if err != nil {
			return c.Status(400).SendString(err.Error())
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return c.Status(404).SendString("Department not found")
		}
		return c.SendString("Deleted")
	})

	// GET /master/locations — Get list of locations with building and department names
	v1.Get("/master/locations", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT l.id, l.building_id, l.department_id, COALESCE(b.name, 'Unassigned') as building_name, COALESCE(dept.name, 'Unassigned') as department_name, COALESCE(l.room, '') as room
			FROM asset.locations l
			LEFT JOIN asset.buildings b ON l.building_id = b.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			ORDER BY building_name, department_name, room
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		var res []fiber.Map
		for rows.Next() {
			var id int
			var buildingID, departmentID sql.NullInt64
			var bname, dname, room string
			rows.Scan(&id, &buildingID, &departmentID, &bname, &dname, &room)

			label := fmt.Sprintf("%s - %s", bname, dname)
			if room != "" {
				label = fmt.Sprintf("%s (%s)", label, room)
			}

			var building, department interface{}
			if buildingID.Valid {
				building = buildingID.Int64
			}
			if departmentID.Valid {
				department = departmentID.Int64
			}
			res = append(res, fiber.Map{
				"id":              id,
				"building_id":     building,
				"department_id":   department,
				"building_name":   bname,
				"department_name": dname,
				"room":            room,
				"label":           label,
			})
		}
		if res == nil {
			res = []fiber.Map{}
		}
		return c.JSON(res)
	})

	v1.Post("/master/locations", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			BuildingID   *int   `json:"building_id"`
			DepartmentID *int   `json:"department_id"`
			Room         string `json:"room"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).SendString("Invalid location data")
		}
		req.Room = strings.TrimSpace(req.Room)
		if req.BuildingID == nil && req.DepartmentID == nil && req.Room == "" {
			return c.Status(400).SendString("Location details are required")
		}
		if req.DepartmentID != nil {
			var departmentBuilding sql.NullInt64
			if err := db.QueryRow("SELECT building_id FROM asset.departments WHERE id = $1", *req.DepartmentID).Scan(&departmentBuilding); err != nil {
				return c.Status(400).SendString("Department not found")
			}
			if req.BuildingID == nil && departmentBuilding.Valid {
				value := int(departmentBuilding.Int64)
				req.BuildingID = &value
			}
			if req.BuildingID != nil && departmentBuilding.Valid && *req.BuildingID != int(departmentBuilding.Int64) {
				return c.Status(400).SendString("Department does not belong to the selected building")
			}
		}
		if _, err := db.Exec("INSERT INTO asset.locations (building_id, department_id, room) VALUES ($1, $2, $3)", req.BuildingID, req.DepartmentID, req.Room); err != nil {
			return c.Status(400).SendString(err.Error())
		}
		return c.SendString("OK")
	})

	v1.Delete("/master/locations/:id", verifyAdmin, func(c *fiber.Ctx) error {
		result, err := db.Exec("DELETE FROM asset.locations WHERE id = $1", c.Params("id"))
		if err != nil {
			return c.Status(400).SendString(err.Error())
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return c.Status(404).SendString("Location not found")
		}
		return c.SendString("Deleted")
	})

	// --- 8. PRINTERS ---
	v1.Get("/printers", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT p.id, p.printer_name, p.brand, p.model, p.ip, p.printer_type, b.name as building_name, d.name as dept_name, p.last_ink_change, p.total_pages_printed, p.starting_page_count
			FROM asset.printers p
			LEFT JOIN asset.departments d ON p.department_id = d.id
			LEFT JOIN asset.buildings b ON d.building_id = b.id
			ORDER BY p.printer_name
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		var res []fiber.Map
		for rows.Next() {
			var id int
			var name, brand, model, ip, ptype, bname, dname, ink string
			var totalPages, startingPage int
			rows.Scan(&id, &name, &brand, &model, &ip, &ptype, &bname, &dname, &ink, &totalPages, &startingPage)
			res = append(res, fiber.Map{
				"id": id, "name": name, "brand": brand, "model": model, "ip": ip, "type": ptype,
				"building": bname, "department": dname, "last_ink": ink,
				"total_pages_printed": totalPages, "starting_page_count": startingPage,
			})
		}
		return c.JSON(res)
	})

	v1.Post("/printers", verifyAdmin, func(c *fiber.Ctx) error {
		var req struct {
			Name   string `json:"name"`
			Brand  string `json:"brand"`
			Model  string `json:"model"`
			IP     string `json:"ip"`
			Type   string `json:"type"`
			DeptID int    `json:"department_id"`
		}
		c.BodyParser(&req)
		db.Exec("INSERT INTO asset.printers (printer_name, brand, model, ip, printer_type, department_id) VALUES ($1, $2, $3, $4, $5, $6)",
			req.Name, req.Brand, req.Model, req.IP, req.Type, req.DeptID)
		return c.SendString("OK")
	})

	// Update Ink Change Date
	v1.Post("/printers/:id/ink", verifyAdmin, func(c *fiber.Ctx) error {
		id := c.Params("id")

		// Update last_ink_change and set starting_page_count to current total_pages_printed
		db.Exec("UPDATE asset.printers SET last_ink_change = CURRENT_DATE, starting_page_count = total_pages_printed WHERE id = $1", id)

		var totalPages int
		db.QueryRow("SELECT total_pages_printed FROM asset.printers WHERE id = $1", id).Scan(&totalPages)

		db.Exec("INSERT INTO asset.printer_logs (printer_id, event_type, description) VALUES ($1, 'INK_CHANGE', $2)",
			id, fmt.Sprintf("เปลี่ยนหมึกพิมพ์ (มิเตอร์เริ่มต้นใหม่: %d แผ่น)", totalPages))
		return c.SendString("Updated")
	})

	// Get Printer Logs (History)
	v1.Get("/printers/:id/logs", verifyAdmin, func(c *fiber.Ctx) error {
		id := c.Params("id")
		rows, err := db.Query("SELECT id, event_type, description, created_at FROM asset.printer_logs WHERE printer_id = $1 ORDER BY created_at DESC", id)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()
		var logs []fiber.Map
		for rows.Next() {
			var logID int
			var etype, desc, cat string
			rows.Scan(&logID, &etype, &desc, &cat)
			logs = append(logs, fiber.Map{"id": logID, "type": etype, "description": desc, "date": cat})
		}
		return c.JSON(logs)
	})

	// --- 9. SETTINGS ---
	v1.Get("/settings", verifyAdmin, func(c *fiber.Ctx) error {
		rows, _ := db.Query("SELECT key, value FROM asset.system_settings")
		res := make(map[string]string)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var k, v string
				rows.Scan(&k, &v)
				res[k] = v
			}
		}
		return c.JSON(res)
	})

	v1.Post("/settings", verifyAdmin, func(c *fiber.Ctx) error {
		var req map[string]string
		c.BodyParser(&req)
		for k, v := range req {
			db.Exec("INSERT INTO asset.system_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value", k, v)
		}
		return c.SendString("Saved")
	})

	v1.Post("/test-telegram", verifyAdmin, func(c *fiber.Ctx) error {
		msg := "🤖 <b>Yuna Asset Collector — Telegram Connection Test</b>\n\n✅ การเชื่อมต่อ Telegram Notification สำเร็จเรียบร้อยค่ะ!\nพร้อมส่งการแจ้งเตือนจากระบบ IT Asset Agent"
		sendTelegram(msg)
		return c.JSON(fiber.Map{"status": "ok", "message": "Test message sent to Telegram"})
	})

	// --- 10. REPORTS & EXPORT ---
	// 1. Devices Inventory Export
	v1.Get("/export/devices", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT d.hostname, d.ip_address, d.mac_address, d.device_type, d.status, d.last_user, 
			       d.bios_serial, COALESCE(b.name, 'N/A') as bname, COALESCE(dept.name, 'N/A') as dname, d.last_seen
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			LEFT JOIN asset.buildings b ON dept.building_id = b.id
			ORDER BY d.hostname
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=computer_inventory_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Hostname", "IP Address", "MAC Address", "Type", "Status", "Last User", "BIOS Serial", "Building", "Department", "Last Seen"})

		for rows.Next() {
			var h, ip, mac, dt, st, u, bs, bname, dname, ls string
			rows.Scan(&h, &ip, &mac, &dt, &st, &u, &bs, &bname, &dname, &ls)
			writer.Write([]string{h, ip, mac, dt, st, u, bs, bname, dname, ls})
		}
		writer.Flush()
		return nil
	})

	// 2. Software Inventory Export
	v1.Get("/export/software", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT name, publisher, version, COUNT(device_id) as install_count
			FROM asset.software_inventory
			GROUP BY name, publisher, version
			ORDER BY install_count DESC, name ASC
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=software_inventory_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Software Name", "Publisher", "Version", "Installed Count"})

		for rows.Next() {
			var name, pub, ver string
			var count int
			rows.Scan(&name, &pub, &ver, &count)
			writer.Write([]string{name, pub, ver, fmt.Sprintf("%d", count)})
		}
		writer.Flush()
		return nil
	})

	// 3. Printers Usage Export
	v1.Get("/export/printers", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT p.printer_name, p.brand, p.model, p.ip, p.printer_type, 
			       COALESCE(b.name, 'N/A') as bname, COALESCE(dept.name, 'N/A') as dname, 
			       p.total_pages_printed, COALESCE(p.last_ink_change::text, 'Never')
			FROM asset.printers p
			LEFT JOIN asset.departments dept ON p.department_id = dept.id
			LEFT JOIN asset.buildings b ON dept.building_id = b.id
			ORDER BY p.printer_name
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=printer_registry_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Printer Name", "Brand", "Model", "Port / IP", "Type", "Building", "Department", "Total Pages Printed", "Last Ink Change"})

		for rows.Next() {
			var name, brand, model, ip, ptype, bname, dname, lastInk string
			var totalPages int
			rows.Scan(&name, &brand, &model, &ip, &ptype, &bname, &dname, &totalPages, &lastInk)
			writer.Write([]string{name, brand, model, ip, ptype, bname, dname, fmt.Sprintf("%d", totalPages), lastInk})
		}
		writer.Flush()
		return nil
	})

	// 4. Security Audit Export
	v1.Get("/export/security", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT d.hostname, d.ip_address, d.windows_build, 
			       COALESCE(d.os_info->>'caption', 'N/A') as os_caption,
			       COALESCE(sp.product_name, 'No active AV reported') as security_product,
			       COALESCE(sp.vendor, '') as security_vendor,
			       COALESCE(sp.realtime_protection_enabled::text, f.facts->'defender'->>'realtime_protection_enabled', 'false') as realtime_protection,
			       COALESCE(sp.signature_status, 'unknown') as signature_status,
			       d.status, d.last_seen
			FROM asset.devices d
			LEFT JOIN asset.device_facts f ON d.uuid = f.device_id
			LEFT JOIN LATERAL (
				SELECT product_name, vendor, realtime_protection_enabled, signature_status
				FROM asset.security_products WHERE device_id = d.uuid
				ORDER BY realtime_protection_enabled DESC, detected_at DESC LIMIT 1
			) sp ON TRUE
			ORDER BY d.hostname
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=security_audit_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Hostname", "IP Address", "OS Caption", "Windows Build", "Security Product", "Vendor", "Realtime Protection", "Signature Status", "Status", "Last Seen"})

		for rows.Next() {
			var h, ip, winBuild, osCaption, product, vendor, realtime, signature, st string
			var lastSeen *time.Time
			rows.Scan(&h, &ip, &winBuild, &osCaption, &product, &vendor, &realtime, &signature, &st, &lastSeen)
			lastSeenText := ""
			if lastSeen != nil {
				lastSeenText = lastSeen.Format(time.RFC3339)
			}
			writer.Write([]string{h, ip, osCaption, winBuild, product, vendor, realtime, signature, st, lastSeenText})
		}
		writer.Flush()
		return nil
	})

	// 5. Storage Health Export
	v1.Get("/export/storage-health", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT d.hostname, d.ip_address, h.storage_info
			FROM asset.devices d
			JOIN asset.hardware_inventory h ON d.uuid = h.device_id
			ORDER BY d.hostname
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=storage_health_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Hostname", "IP Address", "Disk Model", "Size (GB)", "Interface", "Serial Number", "SMART Predict Failure"})

		for rows.Next() {
			var h, ip, storageJSON string
			rows.Scan(&h, &ip, &storageJSON)

			var diskList []map[string]interface{}
			_ = json.Unmarshal([]byte(storageJSON), &diskList)

			for _, disk := range diskList {
				model := fmt.Sprintf("%v", disk["model"])
				sizeGB := fmt.Sprintf("%v", disk["size_gb"])
				iface := fmt.Sprintf("%v", disk["interface"])
				serial := fmt.Sprintf("%v", disk["serial"])
				predFail := "OK"
				if pf, ok := disk["predict_failure"].(bool); ok && pf {
					predFail = "⚠️ PREDICT FAILURE ALERT"
				}
				writer.Write([]string{h, ip, model, sizeGB, iface, serial, predFail})
			}
		}
		writer.Flush()
		return nil
	})

	// 6. Asset Warranty Export
	v1.Get("/export/warranty", verifyAdmin, func(c *fiber.Ctx) error {
		rows, err := db.Query(`
			SELECT d.hostname, COALESCE(d.asset_number, 'N/A'), COALESCE(da.sku, 'N/A'), 
			       COALESCE(da.type_ict, 'N/A'), COALESCE(da.purchase_date::text, '-'), 
			       COALESCE(da.warranty_months, 0), COALESCE(da.warranty_expire::text, '-'), 
			       COALESCE(da.amount, 0), COALESCE(da.comment, '')
			FROM asset.devices d
			LEFT JOIN asset.device_assets da ON d.uuid = da.device_id
			ORDER BY d.hostname
		`)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		defer rows.Close()

		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=asset_warranty_report.csv")

		writer := csv.NewWriter(c)
		writer.Write([]string{"Hostname", "Asset Number", "SKU", "ICT Type", "Purchase Date", "Warranty (Months)", "Warranty Expiration", "Amount (THB)", "Comment"})

		for rows.Next() {
			var h, assetNo, sku, typeICT, purchaseDate, warrantyExpire, comment string
			var months int
			var amount float64
			rows.Scan(&h, &assetNo, &sku, &typeICT, &purchaseDate, &months, &warrantyExpire, &amount, &comment)
			writer.Write([]string{h, assetNo, sku, typeICT, purchaseDate, fmt.Sprintf("%d", months), warrantyExpire, fmt.Sprintf("%.2f", amount), comment})
		}
		writer.Flush()
		return nil
	})

	// Remote VNC trigger endpoint
	v1.Post("/remote/vnc", func(c *fiber.Ctx) error {
		var req struct {
			IP string `json:"ip"`
		}
		if err := c.BodyParser(&req); err != nil || req.IP == "" {
			return c.Status(400).JSON(fiber.Map{"error": "IP address is required"})
		}

		vncPath := `C:\Program Files\RealVNC\VNC4\vncviewer.exe`
		psCmd := exec.Command("powershell.exe", "-WindowStyle", "Hidden", "-Command", fmt.Sprintf("Set-Clipboard -Value '%s'", req.IP))
		_ = psCmd.Run()

		if _, err := os.Stat(vncPath); err == nil {
			cmd := exec.Command(vncPath, req.IP)
			if err := cmd.Start(); err != nil {
				log.Printf("[VNC] Failed to launch VNC Viewer: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to launch VNC Viewer", "details": err.Error()})
			}
		} else {
			log.Printf("[VNC] Executable not found at %s", vncPath)
		}

		return c.JSON(fiber.Map{
			"status":  "success",
			"message": fmt.Sprintf("Triggered RealVNC for IP %s", req.IP),
		})
	})

	// --- 6. Background Jobs (Cron) ---
	cJobs := cron.New()

	// Offline checker every 15 mins (threshold 8 hours)
	cJobs.AddFunc("@every 15m", func() {
		fmt.Println("[Job] Checking for offline devices (threshold: 8 hours)...")
		rows, err := db.Query(`
			SELECT uuid, hostname, ip_address, COALESCE(last_user, 'N/A'), last_seen
			FROM asset.devices
			WHERE status = 'ONLINE' AND last_seen < (CURRENT_TIMESTAMP - INTERVAL '8 hours')
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var devUUID, host, ip, user string
				var lastSeen time.Time
				if err := rows.Scan(&devUUID, &host, &ip, &user, &lastSeen); err == nil {
					db.Exec("UPDATE asset.devices SET status = 'OFFLINE' WHERE uuid = $1", devUUID)
					msg := fmt.Sprintf("🔴 <b>Agent Offline Alert!</b>\n🖥 Host: <b>%s</b>\n🌐 IP: <code>%s</code>\n👤 User: %s\n⏰ ขาดการส่งข้อมูลเกิน 8 ชั่วโมง (Last Seen: %s)",
						host, ip, user, lastSeen.Format("02 Jan 15:04"))
					go sendTelegram(msg)
				}
			}
		}
	})

	// Overtime checker at 20:00 (8:00 PM)
	cJobs.AddFunc("0 20 * * *", func() {
		fmt.Println("[Job] Checking for overtime devices...")
		rows, err := db.Query(`
			SELECT d.hostname, d.ip_address, COALESCE(dept.name, 'Unassigned') as dept_name
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			WHERE d.status = 'ONLINE' AND d.allow_overtime = FALSE
		`)
		if err != nil {
			fmt.Println("[Job] Overtime checker query error:", err)
			return
		}
		defer rows.Close()

		var lines []string
		totalCount := 0
		for rows.Next() {
			var h, ip, dept string
			rows.Scan(&h, &ip, &dept)
			lines = append(lines, fmt.Sprintf("%s (IP: %s) [%s]", h, ip, dept))
			totalCount++
		}
		if totalCount > 0 {
			msg := fmt.Sprintf("🚨 <b>After-Hours Power Activity Report</b>\n⏰ Check Time: 20:00 PM\n🖥 Active Nodes Detected (%d total):\n• %s\n\n🔗 <a href=\"http://127.0.0.1:8080/?view=devices\">Click here to view Dashboard Management Control</a>", totalCount, strings.Join(lines, "\n• "))
			fmt.Println("[Telegram] Sending alert:", msg)
			go sendTelegram(msg)
		}
	})

	// Daily Morning Executive Report at 09:00 AM
	cJobs.AddFunc("0 9 * * *", func() {
		fmt.Println("[Job] Generating Daily Morning Report...")

		var totalDev, onlineDev, offlineDev int
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices").Scan(&totalDev)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices WHERE status = 'ONLINE'").Scan(&onlineDev)
		_ = db.QueryRow("SELECT COUNT(*) FROM asset.devices WHERE status = 'OFFLINE'").Scan(&offlineDev)

		// Group online devices by Department
		rows, err := db.Query(`
			SELECT COALESCE(dept.name, 'ไม่ระบุแผนก') as dept_name, COUNT(d.uuid) as active_count
			FROM asset.devices d
			LEFT JOIN asset.locations l ON d.location_id = l.id
			LEFT JOIN asset.departments dept ON l.department_id = dept.id
			WHERE d.status = 'ONLINE'
			GROUP BY COALESCE(dept.name, 'ไม่ระบุแผนก')
			ORDER BY active_count DESC
		`)

		var deptLines []string
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var deptName string
				var count int
				rows.Scan(&deptName, &count)
				deptLines = append(deptLines, fmt.Sprintf("• %s: <b>%d</b> เครื่อง", deptName, count))
			}
		}

		deptSummary := ""
		if len(deptLines) > 0 {
			deptSummary = "\n\n🏢 <b>จำแนกตามแผนกที่เปิดใช้งาน:</b>\n" + strings.Join(deptLines, "\n")
		}

		todayStr := time.Now().Format("02/01/2006")
		msg := fmt.Sprintf(
			"☀️ <b>Yuna Asset — Daily Morning Report</b> 🌸\n📅 วันที่: <b>%s</b> (09:00 น.)\n\n💻 <b>สรุปสถานะคอมพิวเตอร์วันนี้:</b>\n🟢 เปิดใช้งาน (Online): <b>%d</b> / %d เครื่อง\n🔴 ปิดเครื่อง (Offline): <b>%d</b> เครื่อง%s\n\n🔗 <a href=\"http://127.0.0.1:8080/?view=devices\">เข้าสู่ระบบ Dashboard</a>",
			todayStr, onlineDev, totalDev, offlineDev, deptSummary,
		)
		fmt.Println("[Telegram] Sending morning report...")
		go sendTelegram(msg)
	})

	// Screenshot 90-day retention cleanup every day at 3:00 AM
	cJobs.AddFunc("0 3 * * *", func() {
		res, err := db.Exec("DELETE FROM asset.screenshots WHERE created_at < CURRENT_TIMESTAMP - INTERVAL '90 days'")
		if err != nil {
			fmt.Println("[Job] Screenshot 90-day retention cleanup error:", err)
		} else {
			affected, _ := res.RowsAffected()
			if affected > 0 {
				fmt.Printf("[Job] Deleted %d expired screenshot(s) older than 90 days.\n", affected)
			}
		}
	})

	cJobs.Start()
	go startTelegramBot()

	// --- 7. Static Files (ปรับปรุงแก้ไขให้ถอยโฟลเดอร์แบบปลอดภัย) ---
	baseDir, _ := os.Getwd()
	var webDir string
	if _, err := os.Stat(filepath.Join(baseDir, "web")); err == nil {
		webDir = filepath.Join(baseDir, "web")
	} else if _, err := os.Stat(filepath.Join(filepath.Dir(baseDir), "web")); err == nil {
		webDir = filepath.Join(filepath.Dir(baseDir), "web")
	} else {
		webDir = "web"
	}

	fmt.Printf("[*] Serving Web UI from: %s\n", webDir)

	// แนะนำให้ใช้ Static คลุมหน้าเว็บไปเลยค่ะ Fiber จะจัดการเรื่องรูทพื้นฐานให้เอง
	app.Static("/", webDir)

	// ส่วนตรงนี้ถ้าพี่แบงค์อยากดักหน้าแรกไว้เปิด index.html เผื่อโฟลเดอร์เปลี่ยนก็ใช้แบบนี้ค่ะ
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(webDir, "index.html"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("[*] Yuna Asset Server running on http://0.0.0.0:%s\n", port)
	log.Fatal(app.Listen(":" + port))

}
