import os
import time
import subprocess
import sys

def main():
    print("[Updater] Starting update process...")
    time.sleep(2) # รอให้ Agent หลักปิดตัวลง
    
    agent_file = "agent.py"
    new_agent_file = "agent_new.py"
    
    if os.path.exists(new_agent_file):
        try:
            if os.path.exists(agent_file):
                os.remove(agent_file)
            os.rename(new_agent_file, agent_file)
            print("[Updater] Update applied successfully.")
            
            # รัน Agent ใหม่
            subprocess.Popen([sys.executable, agent_file])
            print("[Updater] Agent restarted. Exiting.")
        except Exception as e:
            print(f"[Updater] Error: {e}")
    else:
        print("[Updater] New agent file not found.")

if __name__ == "__main__":
    main()
