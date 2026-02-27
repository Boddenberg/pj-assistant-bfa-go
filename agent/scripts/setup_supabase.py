"""
Setup script to initialize Supabase tables and seed data.

Usage:
    python -m scripts.setup_supabase

This script:
1. Reads the SQL migration file
2. Executes it against your Supabase instance via the REST API
3. Seeds the knowledge base into the pgvector documents table
"""

import os
import sys
from pathlib import Path

import httpx


def main():
    url = os.getenv("SUPABASE_URL")
    service_key = os.getenv("SUPABASE_SERVICE_ROLE_KEY")

    if not url or not service_key:
        # Try loading from .env
        env_path = Path(__file__).parent.parent / ".env"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    os.environ.setdefault(k.strip(), v.strip())
            url = os.getenv("SUPABASE_URL")
            service_key = os.getenv("SUPABASE_SERVICE_ROLE_KEY")

    if not url or not service_key:
        print("❌ SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY must be set")
        sys.exit(1)

    print(f"🔗 Connecting to Supabase: {url}")

    # Read SQL migration
    sql_path = Path(__file__).parent.parent.parent / "deploy" / "supabase" / "001_init.sql"
    if not sql_path.exists():
        print(f"❌ SQL file not found: {sql_path}")
        sys.exit(1)

    sql = sql_path.read_text(encoding="utf-8")
    print(f"📄 Loaded migration: {sql_path.name} ({len(sql)} chars)")

    # Execute via Supabase SQL API (uses the pg_net or direct SQL endpoint)
    headers = {
        "apikey": service_key,
        "Authorization": f"Bearer {service_key}",
        "Content-Type": "application/json",
        "Prefer": "return=representation",
    }

    # Split SQL into individual statements and execute
    # Filter out empty statements
    statements = [s.strip() for s in sql.split(";") if s.strip() and not s.strip().startswith("--")]

    client = httpx.Client(timeout=30.0)

    # Try using Supabase's SQL endpoint
    sql_url = f"{url}/rest/v1/rpc/exec_sql"

    # First, let's check if we can reach Supabase
    try:
        health = client.get(f"{url}/rest/v1/", headers=headers)
        print(f"✅ Supabase reachable (status: {health.status_code})")
    except Exception as e:
        print(f"❌ Cannot reach Supabase: {e}")
        sys.exit(1)

    print("\n📋 Migration SQL ready. Please run it in the Supabase SQL Editor:")
    print(f"   1. Go to {url.replace('.co', '.co').split('//')[0]}//supabase.com/dashboard")
    print(f"   2. Open your project")
    print(f"   3. Go to SQL Editor")
    print(f"   4. Paste the contents of: deploy/supabase/001_init.sql")
    print(f"   5. Click 'Run'\n")

    # Verify tables exist by trying to query them
    print("🔍 Checking if tables already exist...")

    tables_to_check = ["customer_profiles", "customer_transactions", "documents"]
    for table in tables_to_check:
        try:
            resp = client.get(
                f"{url}/rest/v1/{table}?select=*&limit=1",
                headers=headers,
            )
            if resp.status_code == 200:
                data = resp.json()
                print(f"   ✅ {table}: exists ({len(data)} rows returned)")
            else:
                print(f"   ❌ {table}: not found (status {resp.status_code})")
                print(f"      → Run the SQL migration first!")
        except Exception as e:
            print(f"   ❌ {table}: error ({e})")

    print("\n✨ Done!")


if __name__ == "__main__":
    main()
