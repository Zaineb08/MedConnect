#!/bin/bash

# Define paths
ROOT_DIR=$(pwd)
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_DIR="$ROOT_DIR/backend"

echo "======================================================"
echo " 🚀 Starting up MedConnect Oriental Local Environment "
echo "======================================================"
echo ""

# 1. Set up trapped cleanup function upon Ctrl+C
cleanup() {
    echo ""
    echo "🛑 Shutting down frontend and backend processes..."
    kill $BACKEND_PID $FRONTEND_PID 2>/dev/null
    
    echo "🐳 Do you want to stop the Docker containers too? (y/N)"
    read -t 5 -n 1 answer
    if [[ $answer =~ ^[Yy]$ ]]; then
        echo ""
        docker-compose down
    fi
    
    echo ""
    echo "👋 MedConnect local environment stopped safely."
    exit 0
}

trap cleanup SIGINT SIGTERM

# 2. Check for port conflicts and start necessary Docker services
echo "🔍 Checking for port conflicts..."

echo "⚠️  Checking ports 5432 (Postgres), 8080 (Evolution API), 11434 (Ollama)..."
if lsof -Pi :5432 -sTCP:LISTEN -t >/dev/null ; then
    echo "⚠️  [WARNING] Port 5432 is already in use by another process."
fi
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null ; then
    echo "⚠️  [WARNING] Port 8080 is already in use by another process."
fi
if lsof -Pi :11434 -sTCP:LISTEN -t >/dev/null ; then
    echo "⚠️  [WARNING] Port 11434 is already in use by another process."
fi

# Check if .env exists, if not create from .env.example
if [ ! -f "$ROOT_DIR/.env" ]; then
    echo "📝 Creating .env file from .env.example..."
    cp "$ROOT_DIR/.env.example" "$ROOT_DIR/.env"
    echo "⚠️  IMPORTANT: Please edit .env and set WA_TOKEN before continuing!"
    echo "   WA_TOKEN should be the API key from your Evolution API instance."
fi

# Export environment variables from .env
set -a
source "$ROOT_DIR/.env"
set +a

# Set defaults if not set
WA_URL=${WA_URL:-http://localhost:8080}
WA_TOKEN=${WA_TOKEN:-evolution-secret-key}
WA_INSTANCE=${WA_INSTANCE:-medconnect}
OLLAMA_MODEL=${OLLAMA_MODEL:-llama3:8b-instruct-q4_K_M}

echo "🐳 1/5 Starting Docker dependencies (Postgres, Ollama, Evolution API)..."
docker-compose up -d postgres ollama evolution-api

echo "🧠 Ensuring quantized Ollama model is available: $OLLAMA_MODEL"
if docker exec medconnect_ai ollama pull "$OLLAMA_MODEL" > /dev/null 2>&1; then
    echo "✅ Ollama model ready: $OLLAMA_MODEL"
else
    echo "⚠️  [WARNING] Failed to pre-pull model $OLLAMA_MODEL now. It may be pulled on first request."
fi

# Wait for database to be ready
echo "⏳ Waiting for Database to be healthy..."
for i in {1..20}; do
    if docker exec medconnect_db pg_isready -U medadmin > /dev/null 2>&1; then
        echo "✅ Database is ready!"
        break
    fi
    if [ $i -eq 20 ]; then
        echo "❌ [ERROR] Database failed to become ready in 20 seconds."
        exit 1
    fi
    sleep 1
done

# Wait for Evolution API to be ready
echo "⏳ Waiting for Evolution API to be ready..."
for i in {1..30}; do
    if curl -s "$WA_URL/health" > /dev/null 2>&1; then
        echo "✅ Evolution API is ready!"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "⚠️  [WARNING] Evolution API may not be ready yet."
        echo "   You can continue - it might take a moment to start."
    fi
    sleep 1
done

# 3. Start Backend server
cd "$BACKEND_DIR" || exit
# Create logs directory if it doesn't exist
mkdir -p "$ROOT_DIR/logs"

# Set environment variables for backend
export WA_URL="$WA_URL"
export WA_TOKEN="$WA_TOKEN"
export WA_INSTANCE="$WA_INSTANCE"

# 3. Backend Setup & Run
echo "⚙️  2/5 Starting Go Backend API..."
# Ensure dependencies are available before starting
go mod tidy
go run ./cmd/server/main.go > "$ROOT_DIR/logs/backend.log" 2>&1 &
BACKEND_PID=$!

# Wait exactly 2 seconds to let the backend fully bind to the port
sleep 2

# 4. Frontend Setup & Run
echo "💻 3/5 Starting React Frontend..."
cd "$ROOT_DIR/frontend" || exit
# Check for node_modules
if [ ! -d "node_modules" ]; then
    echo "📦 Initializing frontend dependencies..."
    npm install
fi

npm run dev > "$ROOT_DIR/logs/frontend.log" 2>&1 &
FRONTEND_PID=$!

echo ""
echo "======================================================"
echo " ✅ MedConnect is starting up!"
echo "======================================================"
echo ""
echo "📍 Service URLs:"
echo "   • Frontend:     http://localhost:5173"
echo "   • Backend API:  http://localhost:3000"
echo "   • Ollama AI:   http://localhost:11434"
echo "   • Evolution API (WhatsApp): http://localhost:8080"
echo "   • Evolution Manager (Dashboard): http://localhost:8081"
echo ""
echo "📝 Next Steps:"
echo "   1. Connect WhatsApp to Evolution Manager:"
echo "      • Open http://localhost:8081"
echo "      • Login with API key: $WA_TOKEN"
echo "      • Create instance: $WA_INSTANCE"
echo "      • Scan QR code with your WhatsApp"
echo ""
echo "   2. If using WhatsApp, update WA_TOKEN in .env"
echo "      with your instance API key"
echo ""
echo "🛑 Press Ctrl+C to stop all services"
echo ""

# Keep the script running
wait
