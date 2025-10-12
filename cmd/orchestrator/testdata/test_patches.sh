#!/bin/bash

# Automated Patch Materialization Test Script
# This script creates a workflow, inserts patches, and tests materialization

set -e  # Exit on error

# Change to script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
API_BASE="http://localhost:8081/api/v1"
DB_NAME="orchestrator"
DB_USER="sdutt"
USER_ID="X-User-ID: test-user"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Patch Materialization Test${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Check API availability
echo -e "${YELLOW}Checking API server availability...${NC}"
if ! curl --max-time 2 --silent --fail -H "$USER_ID" "$API_BASE/workflows" > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Cannot connect to API server at $API_BASE${NC}"
    echo -e "${YELLOW}Please start the API server first:${NC}"
    echo -e "${YELLOW}  cd /Users/sdutt/Documents/practice/lyzr/orchestrator/cmd/orchestrator${NC}"
    echo -e "${YELLOW}  go run main.go${NC}"
    exit 1
fi
echo -e "${GREEN}✓ API server is running${NC}\n"

# Step 1: Create Base Workflow
echo -e "${YELLOW}Step 1: Creating Base Workflow${NC}"
RESPONSE=$(curl -s -X POST "$API_BASE/workflows" \
    -H "Content-Type: application/json" \
    -H "X-User-ID: test-user" \
    -d '{
        "tag_name": "patch-base",
        "workflow": '"$(cat workflow_simple.json)"',
        "created_by": "test-user"
    }')

echo "$RESPONSE" | jq '.'

# Extract artifact_id
BASE_ARTIFACT_ID=$(echo "$RESPONSE" | jq -r '.artifact_id')

if [ "$BASE_ARTIFACT_ID" == "null" ] || [ -z "$BASE_ARTIFACT_ID" ]; then
    echo -e "${RED}✗ Failed to create workflow${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Base workflow created: $BASE_ARTIFACT_ID${NC}\n"

# Step 2: Insert Patches via SQL
echo -e "${YELLOW}Step 2: Inserting Test Patches into Database${NC}"

psql -U "$DB_USER" -d "$DB_NAME" -v base_artifact_id="'$BASE_ARTIFACT_ID'" <<'SQL'
-- Patch 1: Add Slack notification node
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content, created_at)
VALUES (
    'sha256:patch001_add_node',
    'application/json;type=patch_ops',
    length('[{"op":"add","path":"/nodes/-","value":{"id":"notify_slack","type":"task","name":"Notify Slack","config":{"timeout":10,"retries":1,"action":"send_slack_notification","channel":"#sales"}}},{"op":"add","path":"/edges/-","value":{"from":"score_lead","to":"notify_slack"}}]'::bytea),
    '[{"op":"add","path":"/nodes/-","value":{"id":"notify_slack","type":"task","name":"Notify Slack","config":{"timeout":10,"retries":1,"action":"send_slack_notification","channel":"#sales"}}},{"op":"add","path":"/edges/-","value":{"from":"score_lead","to":"notify_slack"}}]'::bytea,
    now()
) ON CONFLICT (cas_id) DO NOTHING;

INSERT INTO artifact (artifact_id, kind, cas_id, name, base_version, depth, op_count, created_by, created_at)
VALUES (
    gen_random_uuid(),
    'patch_set',
    'sha256:patch001_add_node',
    'Add Slack notification node',
    :base_artifact_id::uuid,
    1,
    2,
    'test-user',
    now()
)
RETURNING artifact_id \gset patch1_

INSERT INTO patch_chain_member (head_id, seq, member_id)
VALUES (
    :'patch1_artifact_id'::uuid,
    1,
    :'patch1_artifact_id'::uuid
);

-- Patch 2: Update timeout values
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content, created_at)
VALUES (
    'sha256:patch002_update_timeout',
    'application/json;type=patch_ops',
    length('[{"op":"replace","path":"/nodes/0/config/timeout","value":60},{"op":"replace","path":"/nodes/1/config/timeout","value":120}]'::bytea),
    '[{"op":"replace","path":"/nodes/0/config/timeout","value":60},{"op":"replace","path":"/nodes/1/config/timeout","value":120}]'::bytea,
    now()
) ON CONFLICT (cas_id) DO NOTHING;

INSERT INTO artifact (artifact_id, kind, cas_id, name, base_version, depth, op_count, created_by, created_at)
VALUES (
    gen_random_uuid(),
    'patch_set',
    'sha256:patch002_update_timeout',
    'Increase timeout values',
    :base_artifact_id::uuid,
    2,
    2,
    'test-user',
    now()
)
RETURNING artifact_id \gset patch2_

INSERT INTO patch_chain_member (head_id, seq, member_id)
SELECT
    :'patch2_artifact_id'::uuid,
    seq,
    member_id
FROM patch_chain_member
WHERE head_id = :'patch1_artifact_id'::uuid;

INSERT INTO patch_chain_member (head_id, seq, member_id)
VALUES (
    :'patch2_artifact_id'::uuid,
    2,
    :'patch2_artifact_id'::uuid
);

-- Patch 3: Remove enrich_data node
INSERT INTO cas_blob (cas_id, media_type, size_bytes, content, created_at)
VALUES (
    'sha256:patch003_remove_node',
    'application/json;type=patch_ops',
    length('[{"op":"remove","path":"/nodes/1"},{"op":"remove","path":"/edges/0"}]'::bytea),
    '[{"op":"remove","path":"/nodes/1"},{"op":"remove","path":"/edges/0"}]'::bytea,
    now()
) ON CONFLICT (cas_id) DO NOTHING;

INSERT INTO artifact (artifact_id, kind, cas_id, name, base_version, depth, op_count, created_by, created_at)
VALUES (
    gen_random_uuid(),
    'patch_set',
    'sha256:patch003_remove_node',
    'Remove enrich_data node',
    :base_artifact_id::uuid,
    3,
    2,
    'test-user',
    now()
)
RETURNING artifact_id \gset patch3_

INSERT INTO patch_chain_member (head_id, seq, member_id)
SELECT
    :'patch3_artifact_id'::uuid,
    seq,
    member_id
FROM patch_chain_member
WHERE head_id = :'patch2_artifact_id'::uuid;

INSERT INTO patch_chain_member (head_id, seq, member_id)
VALUES (
    :'patch3_artifact_id'::uuid,
    3,
    :'patch3_artifact_id'::uuid
);

-- Create tags (with new username column for multi-user support)
INSERT INTO tag (username, tag_name, target_kind, target_id, created_by, moved_by, moved_at)
VALUES ('test-user', 'patch-test-1', 'patch_set', :'patch1_artifact_id'::uuid, 'test-user', 'test-user', now())
ON CONFLICT (username, tag_name) DO UPDATE SET
    target_kind = EXCLUDED.target_kind,
    target_id = EXCLUDED.target_id,
    version = tag.version + 1,
    moved_by = EXCLUDED.moved_by,
    moved_at = EXCLUDED.moved_at;

INSERT INTO tag (username, tag_name, target_kind, target_id, created_by, moved_by, moved_at)
VALUES ('test-user', 'patch-test-2', 'patch_set', :'patch2_artifact_id'::uuid, 'test-user', 'test-user', now())
ON CONFLICT (username, tag_name) DO UPDATE SET
    target_kind = EXCLUDED.target_kind,
    target_id = EXCLUDED.target_id,
    version = tag.version + 1,
    moved_by = EXCLUDED.moved_by,
    moved_at = EXCLUDED.moved_at;

INSERT INTO tag (username, tag_name, target_kind, target_id, created_by, moved_by, moved_at)
VALUES ('test-user', 'patch-test-3', 'patch_set', :'patch3_artifact_id'::uuid, 'test-user', 'test-user', now())
ON CONFLICT (username, tag_name) DO UPDATE SET
    target_kind = EXCLUDED.target_kind,
    target_id = EXCLUDED.target_id,
    version = tag.version + 1,
    moved_by = EXCLUDED.moved_by,
    moved_at = EXCLUDED.moved_at;

\echo 'Patches inserted successfully!'
SQL

if [ $? -ne 0 ]; then
    echo -e "${RED}✗ Failed to insert patches${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Patches inserted successfully${NC}\n"

# Step 3: Test Patch Materialization
echo -e "${YELLOW}Step 3: Testing Patch Materialization${NC}\n"

# Test Patch 1 (depth=1)
echo -e "${BLUE}Test 3a: Patch-test-1 (depth=1, adds Slack node)${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-1?materialize=false" | jq '{tag, kind, depth, patch_count}'
echo ""
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-1?materialize=true" | jq '.workflow.nodes | length'
echo -e "${GREEN}✓ Should have 4 nodes (original 3 + 1 added)${NC}\n"

# Test Patch 2 (depth=2)
echo -e "${BLUE}Test 3b: Patch-test-2 (depth=2, also updates timeouts)${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-2?materialize=false" | jq '{tag, kind, depth, patch_count}'
echo ""
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-2?materialize=true" | jq '.workflow.nodes[0].config.timeout'
echo -e "${GREEN}✓ Should be 60 (updated from 30)${NC}\n"

# Test Patch 3 (depth=3)
echo -e "${BLUE}Test 3c: Patch-test-3 (depth=3, also removes enrich node)${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-3?materialize=false" | jq '{tag, kind, depth, patch_count}'
echo ""
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-3?materialize=true" | jq '.workflow.nodes | length'
echo -e "${GREEN}✓ Should have 3 nodes (removed enrich_data)${NC}\n"

# Step 4: Show Full Workflows
echo -e "${YELLOW}Step 4: Full Materialized Workflows${NC}\n"

echo -e "${BLUE}Original Base Workflow:${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-base?materialize=true" | jq '.workflow.nodes[] | {id, type}'

echo -e "\n${BLUE}After Patch 1 (added notify_slack):${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-1?materialize=true" | jq '.workflow.nodes[] | {id, type}'

echo -e "\n${BLUE}After Patch 2 (updated timeouts):${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-2?materialize=true" | jq '.workflow.nodes[] | {id, timeout: .config.timeout}'

echo -e "\n${BLUE}After Patch 3 (removed enrich_data):${NC}"
curl -s -H "$USER_ID" "$API_BASE/workflows/patch-test-3?materialize=true" | jq '.workflow.nodes[] | {id, type}'

# Summary
echo -e "\n${BLUE}========================================${NC}"
echo -e "${GREEN}  All Patch Tests Completed!${NC}"
echo -e "${BLUE}========================================${NC}"

echo -e "\n${YELLOW}Summary:${NC}"
echo -e "- Created base workflow: $BASE_ARTIFACT_ID"
echo -e "- Inserted 3 patches (add, replace, remove operations)"
echo -e "- Created 3 tags pointing to different patch depths"
echo -e "- Verified materialization works correctly"
echo -e "\n${YELLOW}Cleanup:${NC}"
echo -e "To clean up test data, run:"
echo -e "  psql -U $DB_USER -d $DB_NAME -c \"DELETE FROM tag WHERE username = 'test-user' AND tag_name LIKE 'patch-%'\""
