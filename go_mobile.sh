#!/bin/bash

# Function to handle script exit and kill background processes
cleanup() {
    echo ""
    echo "🛑 Shutting down OpenCode and Ngrok..."
    if [ -n "$OPENCODE_PID" ]; then
        kill "$OPENCODE_PID" 2>/dev/null
    fi
    if [ -n "$NGROK_PID" ]; then
        kill "$NGROK_PID" 2>/dev/null
    fi
    echo "✅ Done."
    exit
}

# Trap interrupt signals (Ctrl+C) to run cleanup
trap cleanup SIGINT SIGTERM

echo "🚀 Starting OpenCode Mobile Mode..."

# 1. Start OpenCode Web Server in background
# We bind to 0.0.0.0 to ensure it listens for the tunnel
# Using a log file prevents output clutter
echo "   - Launching OpenCode Web Server (Port 3000)..."
opencode web --port 3000 --hostname 0.0.0.0 > opencode_web.log 2>&1 &
OPENCODE_PID=$!

# Wait a moment for OpenCode to start
sleep 2

# 2. Start Ngrok Tunnel in background
echo "   - establishing Ngrok Secure Tunnel..."
ngrok http 3000 > ngrok.log 2>&1 &
NGROK_PID=$!

# Wait for Ngrok to initialize and generate URL
echo "   - Waiting for tunnel URL..."
sleep 4

# 3. Retrieve and Display the Public URL
# We use the local Ngrok API to find the assigned public URL
TUNNEL_URL=$(curl -s http://127.0.0.1:4040/api/tunnels | jq -r '.tunnels[0].public_url')

if [ "$TUNNEL_URL" == "null" ] || [ -z "$TUNNEL_URL" ]; then
    echo "❌ Error: Could not retrieve Ngrok URL."
    echo "   Check 'ngrok.log' or 'opencode_web.log' for details."
    cleanup
else
    echo ""
    echo "🎉 SUCCESS! Your mobile coding environment is ready."
    echo "========================================================"
    echo "📲 Open this URL on your mobile browser:"
    echo ""
    echo "   $TUNNEL_URL"
    echo ""
    echo "========================================================"
    echo "ℹ️  Logs are being saved to 'opencode_web.log' and 'ngrok.log'"
    echo "YOUR SESSION IS LIVE. Press Ctrl+C to stop servers."
    
    # Wait indefinitely so the script doesn't exit
    wait
fi
