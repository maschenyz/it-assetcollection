import sys
from datetime import datetime

def info(tag, message):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] [{tag}] {message}")
    sys.stdout.flush()

def error(tag, message):
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] [{tag}] ERROR: {message}", file=sys.stderr)
    sys.stderr.flush()
