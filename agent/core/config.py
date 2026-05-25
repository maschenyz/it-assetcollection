import os
import json

# CONFIG_PATH is in the agent root directory (c:\project A\agent\agent_config.json)
CONFIG_PATH = os.path.join(os.path.dirname(os.path.dirname(__file__)), "agent_config.json")

def load_config():
    defaults = {
        "server_url": "http://localhost:8080/api/v1",
        "agent_token": "yuna_secret_token_2024",
        "sync_interval_minutes": 30,
        "poll_interval_seconds": 30
    }
    if os.path.exists(CONFIG_PATH):
        try:
            with open(CONFIG_PATH, 'r') as f:
                user_config = json.load(f)
                defaults.update(user_config)
        except:
            pass
    return defaults

def save_config(config_dict):
    try:
        with open(CONFIG_PATH, 'w') as f:
            json.dump(config_dict, f, indent=2)
    except Exception as e:
        print(f"[Config] Error saving config: {e}")
