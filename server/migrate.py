import psycopg2
from dotenv import load_dotenv
import os

load_dotenv()

def run_migration():
    try:
        conn = psycopg2.connect(
            host=os.getenv("DB_HOST", "localhost"),
            database=os.getenv("DB_NAME", "asset"),
            user=os.getenv("DB_USER", "postgres"),
            password=os.getenv("DB_PASSWORD", "12123"),
            port=os.getenv("DB_PORT", "5432")
        )
        conn.autocommit = True
        cur = conn.cursor()
        
        # Read schema.sql
        schema_path = os.path.join(os.path.dirname(__file__), "schema.sql")
        if os.path.exists(schema_path):
            with open(schema_path, "r", encoding="utf-8") as f:
                sql = f.read()
                cur.execute(sql)
                print("Database migration (schema.sql) executed successfully!")
        else:
            print("Error: schema.sql not found!")
            
        cur.close()
        conn.close()
    except Exception as e:
        print(f"Migration Error: {e}")

if __name__ == "__main__":
    run_migration()
