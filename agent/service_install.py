import win32serviceutil
import win32service
import win32event
import servicemanager
import socket
import sys
import os
import time

# นำเข้าฟังก์ชันหลักจาก agent.py
import agent

class AssetAgentService(win32serviceutil.ServiceFramework):
    _svc_name_ = "YunaAssetAgent"
    _svc_display_name_ = "Yuna IT Asset Management Agent"
    _svc_description_ = "Collects hardware and software inventory and syncs with the server."

    def __init__(self, args):
        win32serviceutil.ServiceFramework.__init__(self, args)
        self.hWaitStop = win32event.CreateEvent(None, 0, 0, None)
        self.stop_requested = False

    def SvcStop(self):
        self.ReportServiceStatus(win32service.SERVICE_STOP_PENDING)
        win32event.SetEvent(self.hWaitStop)
        self.stop_requested = True

    def SvcDoRun(self):
        servicemanager.LogMsg(servicemanager.EVENTLOG_INFORMATION_TYPE,
                              servicemanager.PYS_SERVICE_STARTED,
                              (self._svc_name_, ''))
        self.main()

    def main(self):
        config = agent.load_config()
        headers = {"X-Agent-Token": config["agent_token"]}
        
        while not self.stop_requested:
            try:
                payload = agent.collect_payload()
                agent.requests.post(config["server_url"], json=payload, headers=headers)
            except Exception as e:
                pass # ใน Service เราจะไม่ print ออกจอ แต่สามารถใช้ Logging ได้
            
            # รอตามเวลาที่กำหนด หรือจนกว่าจะมีการสั่ง Stop
            rc = win32event.WaitForSingleObject(self.hWaitStop, config["sync_interval_minutes"] * 60 * 1000)
            if rc == win32event.WAIT_OBJECT_0:
                break

if __name__ == '__main__':
    if len(sys.argv) == 1:
        servicemanager.Initialize()
        servicemanager.PrepareToHostSingle(AssetAgentService)
        servicemanager.StartServiceCtrlDispatcher()
    else:
        win32serviceutil.HandleCommandLine(AssetAgentService)
