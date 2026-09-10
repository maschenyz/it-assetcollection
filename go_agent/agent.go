package main

// 1. กฎเหล็กข้อที่ 1: Import แพ็กเกจต้องอยู่บนสุด ห้ามมีอะไรมาคั่น!
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// 2. ฝังค่าคงที่สำหรับระบุตัวตนและที่อยู่ Server (เปลี่ยน IP ตรงนี้ได้เลยค่ะ)
const (
	ServerURL   = "http://127.0.0.1:8080/api/monitor" // แก้เป็น IP Server ของคุณแบงค์นะ
	SecretToken = "yuna_secret_token_2024"            // รหัสลับให้ตรงกับฝั่ง Server .env
)

// 3. หน้าตา JSON โครงสร้างข้อมูล Contract
type AssetPayload struct {
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	CPUUsage  float64 `json:"cpu_usage"`
	RAMTotal  uint64  `json:"ram_total"`
	RAMUsed   uint64  `json:"ram_used"`
	RAMFree   uint64  `json:"ram_free"`
	Timestamp string  `json:"timestamp"`
}

func main() {
	fmt.Printf("[+] Agent started... Target: %s\n", ServerURL)
	fmt.Println("Press Ctrl+C to stop.")

	for {
		// ดึงข้อมูลระบบ
		payload := collectSystemData()

		// แปลงเป็น JSON
		jsonData, err := json.Marshal(payload)
		if err != nil {
			fmt.Println("[-] Error marshalling JSON:", err)
			continue
		}

		// ส่งข้อมูลแนบรหัสลับ
		err = sendData(ServerURL, jsonData)
		if err != nil {
			fmt.Println("[-] Send failed:", err)
		} else {
			fmt.Println("[+] Data sent successfully at:", payload.Timestamp)
		}

		// ส่งทุกๆ 10 วินาที
		time.Sleep(10 * time.Second)
	}
}

// ฟังก์ชันกวาดข้อมูลระบบ
func collectSystemData() AssetPayload {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown-PC"
	}

	cpuPercents, err := cpu.Percent(500*time.Millisecond, false)
	var cpuUsage float64
	if err == nil && len(cpuPercents) > 0 {
		cpuUsage = cpuPercents[0]
	}

	vMem, err := mem.VirtualMemory()
	var ramTotal, ramUsed, ramFree uint64
	if err == nil {
		ramTotal = vMem.Total / 1024 / 1024
		ramUsed = vMem.Used / 1024 / 1024
		ramFree = vMem.Available / 1024 / 1024
	}

	return AssetPayload{
		Hostname:  hostname,
		OS:        runtime.GOOS,
		CPUUsage:  cpuUsage,
		RAMTotal:  ramTotal,
		RAMUsed:   ramUsed,
		RAMFree:   ramFree,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}
}

// ฟังก์ชันส่ง HTTP POST พร้อมแนบ Token ใน Header
func sendData(url string, data []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	// แนบ Header สำคัญส่งไปดักแอนตี้ไวรัสและให้ Server ตรวจตั๋ว
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", SecretToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned status: %s", resp.Status)
	}

	return nil
}
