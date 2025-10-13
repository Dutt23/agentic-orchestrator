# Partition Management Guide

## Overview

The `run` table is partitioned by time (year) to optimize query performance and enable efficient data archival. This document explains how to manage partitions using the provided script.

## Why Partitioning?

- **Query Performance**: Queries filtered by `submitted_at` only scan relevant partitions
- **Data Lifecycle**: Easy archival/deletion of old data by dropping entire partitions
- **Maintenance**: Index rebuilds and vacuums can target specific partitions
- **Scalability**: Distributes data across multiple physical tables

## Partition Management Script

Location: `scripts/manage-partitions.sh`

### Commands

#### List Partitions
```bash
./scripts/manage-partitions.sh list
```
Shows all existing partitions with their date ranges.

#### Create Partition for Specific Year
```bash
./scripts/manage-partitions.sh create 2026
```
Creates partition `run_2026` covering 2026-01-01 to 2027-01-01.

#### Auto-Create Future Partitions
```bash
./scripts/manage-partitions.sh auto 2
```
Creates partitions for current year + next 2 years (default is 2).

**Recommended**: Run this monthly via cron to ensure partitions exist.

#### Show Partition Statistics
```bash
./scripts/manage-partitions.sh stats
```
Displays size, row count, and date ranges for all partitions.

#### Drop Old Partition
```bash
./scripts/manage-partitions.sh drop 2020
```
Drops the specified partition **with confirmation prompt**.
⚠️ **Warning**: This permanently deletes all data in that partition!

## Automation

### Monthly Partition Creation (Recommended)

Add to crontab to automatically create partitions 2 years ahead:

```bash
# Edit crontab
crontab -e

# Add this line (runs on 1st of each month at midnight)
0 0 1 * * cd /path/to/orchestrator && ./scripts/manage-partitions.sh auto 2 >> /var/log/orchestrator-partitions.log 2>&1
```

### Quarterly Partition Cleanup (Optional)

Archive/drop old partitions quarterly:

```bash
# Runs on 1st of Jan, Apr, Jul, Oct at 2 AM
0 2 1 1,4,7,10 * cd /path/to/orchestrator && ./scripts/manage-partitions.sh drop $(date -d '5 years ago' +\%Y) < /dev/null
```

⚠️ **Warning**: The above will auto-delete without confirmation. Test carefully!

## Manual Workflow

### Pre-Production Checklist
Before deploying to production, ensure partitions exist:

```bash
# 1. Check existing partitions
./scripts/manage-partitions.sh list

# 2. Create partitions for current + next 2 years
./scripts/manage-partitions.sh auto 2

# 3. Verify
./scripts/manage-partitions.sh stats
```

### Monitoring

Monitor partition health regularly:

```bash
# Check if current year partition exists
CURRENT_YEAR=$(date +%Y)
psql -d orchestrator -c "SELECT 1 FROM pg_tables WHERE tablename='run_${CURRENT_YEAR}';"

# Alert if missing
if [ $? -ne 0 ]; then
    echo "WARNING: Current year partition missing!"
fi
```

## Troubleshooting

### Partition Does Not Exist Error

**Error**: `no partition of relation "run" found for row`

**Cause**: Trying to insert data for a date that has no partition.

**Fix**:
```bash
# Find the year for the failing date
# Create partition for that year
./scripts/manage-partitions.sh create 2027
```

### Query Performance Issues

**Symptom**: Slow queries even with partitioning.

**Diagnosis**:
```bash
# Check query plan
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM run
WHERE submitted_at >= '2025-01-01'
  AND submitted_at < '2025-02-01';
```

**Look for**: `Partition Pruning` should show only relevant partitions scanned.

### Partition Maintenance

**Scenario**: Partition has grown large and needs maintenance.

```sql
-- Vacuum specific partition
VACUUM ANALYZE run_2025;

-- Rebuild indexes on specific partition
REINDEX TABLE run_2025;
```

## Best Practices

1. **Create Partitions Ahead of Time**
   - Always have partitions for current + 1-2 years ahead
   - Set up automated monthly creation

2. **Archive Before Dropping**
   - Export old partition data before dropping
   ```bash
   pg_dump -d orchestrator -t run_2020 > run_2020_archive.sql
   ./scripts/manage-partitions.sh drop 2020
   ```

3. **Monitor Partition Sizes**
   - Run `stats` command monthly
   - Alert if any partition exceeds threshold (e.g., 10GB)

4. **Default Partition (Future Enhancement)**
   - Consider adding a DEFAULT partition to catch unexpected dates
   ```sql
   CREATE TABLE run_default PARTITION OF run DEFAULT;
   ```

5. **Test in Staging First**
   - Always test partition changes in staging before production
   - Verify data distribution with `stats` command

## SQL Reference

### Manual Partition Creation
```sql
-- Create partition for 2027
CREATE TABLE run_2027 PARTITION OF run
    FOR VALUES FROM ('2027-01-01') TO ('2028-01-01');
```

### Check Partition Info
```sql
-- List all partitions
SELECT
    c.relname AS partition_name,
    pg_get_expr(c.relpartbound, c.oid) AS partition_bounds,
    pg_size_pretty(pg_total_relation_size(c.oid)) AS size
FROM pg_class c
JOIN pg_inherits i ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'run'
ORDER BY c.relname;
```

### Verify Partition Pruning
```sql
-- Should only scan relevant partition(s)
EXPLAIN
SELECT * FROM run
WHERE submitted_at >= '2025-06-01'
  AND submitted_at < '2025-07-01';
```

## Migration Strategy

### Initial Setup (Already Done)
The schema migration `001_final_schema.sql` creates:
- `run_2024` partition (2024-01-01 to 2025-01-01)
- `run_2025` partition (2025-01-01 to 2026-01-01)

### Future Years
Run the auto-creation script regularly:
```bash
# Create partitions for 2026, 2027, 2028
./scripts/manage-partitions.sh auto 3
```

## Monitoring Queries

### Find Partition with Most Data
```sql
SELECT
    c.relname AS partition_name,
    (SELECT COUNT(*) FROM ONLY c.oid) AS row_count
FROM pg_class c
JOIN pg_inherits i ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'run'
ORDER BY row_count DESC
LIMIT 5;
```

### Check for Missing Partitions
```sql
-- Find years with run data but no partition
SELECT DISTINCT
    EXTRACT(YEAR FROM submitted_at) AS year
FROM run
ORDER BY year;

-- Compare with existing partitions
SELECT
    SUBSTRING(relname FROM 'run_(\d+)') AS year
FROM pg_class c
JOIN pg_inherits i ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = 'run';
```

## Related Documentation

- [DESIGN.md](DESIGN.md) - Overall schema design
- [OPERATIONS.md](OPERATIONS.md) - Operational procedures
- [TAG_MOVE_EXPLAINED.md](TAG_MOVE_EXPLAINED.md) - Tag management
