# PostgreSQL Initialization Scripts

This directory contains SQL and shell scripts that will be executed when the PostgreSQL container is first initialized.

## Usage

Place your `.sql` or `.sh` files in this directory. They will be executed in alphabetical order during the first startup of the PostgreSQL container.

## Script Naming Convention

Use numeric prefixes to control execution order:

```
01-custom-indexes.sql
02-custom-functions.sql
03-seed-data.sql
```

## Example Scripts

### Create Custom Indexes

**File: `01-custom-indexes.sql`**
```sql
-- Custom performance indexes for ThinLine Radio
-- These are optional but can improve query performance

\c thinline_radio

-- Index for faster call lookups by timestamp
CREATE INDEX IF NOT EXISTS idx_calls_timestamp 
ON calls(timestamp DESC);

-- Index for faster talkgroup searches
CREATE INDEX IF NOT EXISTS idx_calls_talkgroup_id 
ON calls(talkgroup_id);

-- Index for faster system searches
CREATE INDEX IF NOT EXISTS idx_calls_system_id 
ON calls(system_id);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_calls_system_talkgroup_timestamp 
ON calls(system_id, talkgroup_id, timestamp DESC);
```

### Seed Test Data (Development Only)

**File: `02-seed-test-data.sql`**
```sql
-- WARNING: Only use for development/testing!
-- This will create test data in your database

\c thinline_radio

-- Insert test systems, talkgroups, etc.
-- (Add your test data here)
```

### Custom Functions

**File: `03-custom-functions.sql`**
```sql
-- Custom PostgreSQL functions

\c thinline_radio

-- Example: Function to clean up old calls
CREATE OR REPLACE FUNCTION cleanup_old_calls(days_to_keep INTEGER)
RETURNS INTEGER AS $$
DECLARE
  deleted_count INTEGER;
BEGIN
  DELETE FROM calls 
  WHERE timestamp < NOW() - (days_to_keep || ' days')::INTERVAL;
  
  GET DIAGNOSTICS deleted_count = ROW_COUNT;
  RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;
```

### Shell Script Example

**File: `04-custom-setup.sh`**
```bash
#!/bin/bash
# Custom PostgreSQL setup script

set -e

# This script runs as the postgres user
# Use psql to execute commands

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Your SQL commands here
    SELECT 'Custom setup completed' AS message;
EOSQL

echo "Custom initialization completed"
```

## Important Notes

1. **First Run Only**: Scripts only execute on the **first initialization** of the database. If the PostgreSQL data directory already exists, scripts will NOT run.

2. **To Re-run Scripts**: 
   ```bash
   # Stop containers
   docker-compose down
   
   # Remove PostgreSQL data
   sudo rm -rf docker/data/postgres
   
   # Start again (scripts will run)
   docker-compose up -d
   ```

3. **Database Connection**: Scripts have access to these environment variables:
   - `POSTGRES_DB` - Database name
   - `POSTGRES_USER` - Database user
   - `POSTGRES_PASSWORD` - Database password

4. **Error Handling**: If a script fails, the entire initialization fails and the container will not start. Check logs:
   ```bash
   docker-compose logs postgres
   ```

5. **Script Permissions**: Shell scripts must be executable:
   ```bash
   chmod +x docker/init-db/*.sh
   ```

6. **Test Scripts**: Always test your scripts in a development environment first!

## Default ThinLine Radio Schema

ThinLine Radio automatically creates its own schema during first run. You don't need to create the base tables - they are handled by the application's migration system.

These scripts are for:
- Custom indexes for performance
- Additional functions or procedures
- Test/seed data for development
- Custom extensions or modifications

## Troubleshooting

**Scripts not running?**
- Ensure PostgreSQL data directory is empty on first start
- Check script permissions
- Review logs: `docker-compose logs postgres`

**SQL errors?**
- Test scripts manually: `docker-compose exec postgres psql -U thinline_user thinline_radio -f /docker-entrypoint-initdb.d/your-script.sql`
- Use `\c dbname` to connect to the correct database
- Use `\i filename.sql` to include other SQL files

**Need to debug?**
- Add `set -x` to shell scripts for verbose output
- Add `\echo` statements to SQL scripts
- Check `/var/log/postgresql/` inside the container

