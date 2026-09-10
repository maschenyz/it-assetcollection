package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"go_agent/config"
	"go_agent/logger"
)

// SymbolicMap provides defense-in-depth for symbolic command names
var SymbolicMap = map[string]string{
	"lock_screen":     `rundll32.exe user32.dll,LockWorkStation`,
	"restart_now":     `shutdown /r /f /t 0`,
	"shutdown_now":    `shutdown /s /f /t 0`,
	"shutdown_60":     `shutdown /s /f /t 60`,
	"cancel_shutdown": `shutdown /a`,
	"restart_spooler": `net stop spooler && net start spooler`,
	"clear_temp":      `cmd /c del /q /f /s "%TEMP%\*" & del /q /f /s "C:\Windows\Temp\*"`,
	"flush_dns":       `ipconfig /flushdns`,
	"check_disk":      `chkdsk C: /scan`,
}

type taskResult struct {
	Status string `json:"status"`
	Output string `json:"output"`
}

// ExecuteCommand runs a task script/command and reports the result back to the server.
func ExecuteCommand(taskID int, script string, deviceUUID string) {
	cfg := config.Load()
	headers := map[string]string{"X-Agent-Token": cfg.AgentToken}

	script = strings.TrimSpace(script)

	// Check symbolic map
	if mapped, ok := SymbolicMap[script]; ok {
		logger.Info("Executor", fmt.Sprintf("Mapping '%s' → '%s'", script, mapped))
		script = mapped
	}

	logger.Info("Executor", fmt.Sprintf("Executing task %d: %s", taskID, script))

	var result taskResult

	switch script {
	case "screenshot":
		result.Status = "failed"
		result.Output = "screenshot not supported in Go agent yet"

	case "scan_network":
		result.Status = "failed"
		result.Output = "network scan not implemented yet"

	default:
		res, err := runCommand(script)
		if err != nil {
			result.Status = "failed"
			result.Output = err.Error()
			logger.Error("Executor", fmt.Sprintf("Task %d failed: %v", taskID, err))
		} else {
			result.Status = "success"
			result.Output = res
		}
	}

	reportResult(cfg, taskID, result, headers)
}

func runCommand(script string) (string, error) {
	ctx := exec.Command("cmd", "/C", script)
	var out bytes.Buffer
	ctx.Stdout = &out
	ctx.Stderr = &out
	err := ctx.Run()
	return out.String(), err
}

func reportResult(cfg *config.Config, taskID int, result taskResult, headers map[string]string) {
	url := fmt.Sprintf("%s/agent/tasks/%d/result", cfg.ServerURL, taskID)
	body, _ := json.Marshal(result)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		logger.Error("Executor", fmt.Sprintf("Could not create request for task %d: %v", taskID, err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Executor", fmt.Sprintf("Failed to report task %d result: %v", taskID, err))
		return
	}
	defer resp.Body.Close()
	logger.Info("Executor", fmt.Sprintf("Task %d result reported (status=%s, http=%d)", taskID, result.Status, resp.StatusCode))
}
