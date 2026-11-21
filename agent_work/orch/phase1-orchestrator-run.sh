#!/bin/bash
# Fable orchestrator runner — Analytics Edge Gateway, Phase 1 (plan only).
set -u
cd /root/Software/analytics-edge-gateway || exit 1
export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$HOME/.deno/bin:$HOME/.opencode/bin:/usr/local/go/bin:$PATH"
echo "orchestrator start: $(date -Is)" >> /root/runs/gateway-orchestrator-progress.txt
IS_SANDBOX=1 claude -p --model claude-fable-5-2 --dangerously-skip-permissions \
  < /root/briefs/gateway-orchestrator-mission.md \
  > /root/runs/gateway-orchestrator.log 2>&1
echo $? > /root/runs/gateway-orchestrator.exit
echo "orchestrator exit: $(date -Is) code=$(cat /root/runs/gateway-orchestrator.exit)" >> /root/runs/gateway-orchestrator-progress.txt
sleep 100000
