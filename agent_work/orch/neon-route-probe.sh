#!/bin/bash
set -a; . /root/Software/.secrets/yt-transcribe.env; . /root/Software/.secrets/gemini-neon.env; set +a
export PATH="$HOME/.opencode/bin:$PATH"
echo "direct start $(date -Is)" > /root/runs/gateway-neon-probe.log
timeout 90 opencode run --model google/gemini-4-neon "Reply with exactly: NEON_OK" >> /root/runs/gateway-neon-probe.log 2>&1
echo "direct exit:$?" >> /root/runs/gateway-neon-probe.log
echo "openrouter start $(date -Is)" >> /root/runs/gateway-neon-probe.log
timeout 90 opencode run --model openrouter/google/gemini-4-neon "Reply with exactly: NEON_OK" >> /root/runs/gateway-neon-probe.log 2>&1
echo "openrouter exit:$?" >> /root/runs/gateway-neon-probe.log
echo "done $(date -Is)" >> /root/runs/gateway-neon-probe.log
