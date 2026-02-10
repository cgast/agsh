#!/bin/bash
# orchestrator.sh — connects an LLM to agsh for autonomous operation
#
# Usage: ./orchestrator.sh [--model claude-sonnet-4-20250514] [--spec project.agsh.yaml]
#
# This script:
# 1. Starts agsh in agent mode (JSON-RPC over stdin/stdout)
# 2. Sends the spec to the LLM along with agsh's command catalog
# 3. Relays LLM decisions to agsh and agsh responses back to the LLM
# 4. Logs all interactions to ./logs/

MODEL="${MODEL:-claude-sonnet-4-20250514}"
SPEC="${SPEC:-project.agsh.yaml}"
LOG_DIR="./logs/$(date +%Y%m%d-%H%M%S)"

mkdir -p "$LOG_DIR"

echo "Starting agsh in agent mode..."
echo "Model: $MODEL"
echo "Spec: $SPEC"
echo "Logs: $LOG_DIR"

# System prompt for the LLM orchestrator
SYSTEM_PROMPT=$(cat <<'EOF'
You are an autonomous agent operating inside agsh (agent shell).
You communicate via JSON-RPC. Available methods:

- commands.list: Discover available commands
- commands.describe <name>: Get schema for a command
- project.load <spec>: Load a project spec
- project.plan: Generate an execution plan from the loaded spec
- execute <command> <args>: Run a single command
- pipeline <steps>: Run a multi-step pipeline
- context.get/set: Read/write shared context
- checkpoint.save/restore: Manage state checkpoints
- history: View execution log

Your workflow:
1. Load the project spec
2. Discover available commands with commands.list
3. Generate a plan with project.plan
4. Execute the plan step by step
5. If a step fails, decide: retry, skip, or rollback
6. Verify the final output against success criteria

Always checkpoint before destructive operations.
Always verify after each significant step.
Report your reasoning alongside each action.
EOF
)

# The actual orchestration loop would be implemented here.
# For the prototype, this serves as documentation of the intended flow.
echo "Orchestrator script is a reference implementation."
echo "See the system prompt above for the LLM's operating instructions."
echo ""
echo "To run manually:"
echo "  agsh --mode=agent < commands.jsonl"
echo ""
echo "To run the built-in demo simulation:"
echo "  agsh demo 04"
