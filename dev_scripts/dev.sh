#!/bin/bash
# Orchestrator Platform - Interactive Development Menu

cd "$(dirname "$0")"

show_menu() {
    clear
    echo "╔════════════════════════════════════════════════════╗"
    echo "║     Orchestrator Platform - Dev Menu              ║"
    echo "╚════════════════════════════════════════════════════╝"
    echo ""
    
    # Show current status
    if [ -f pids/supervisor.sock ] && supervisorctl -c supervisord.conf status &> /dev/null; then
        echo "📊 Current Status:"
        supervisorctl -c supervisord.conf status 2>/dev/null | sed 's/^/   /'
    else
        echo "⚠️  Services are not running"
    fi
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  1)  Start all services"
    echo "  2)  Start core (orchestrator + workflow-runner + fanout)"
    echo "  3)  Start backend only (no frontend)"
    echo "  4)  Start frontend only"
    echo "  5)  Stop all services"
    echo ""
    echo "  6)  Restart service"
    echo "  7)  View logs"
    echo "  8)  Check status"
    echo ""
    echo "  0)  Exit"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

start_core() {
    echo "🚀 Starting core services..."
    supervisorctl -c supervisord.conf start orchestrator workflow-runner fanout
    echo "✅ Core services started"
    sleep 2
}

start_backend() {
    echo "🚀 Starting backend services..."
    supervisorctl -c supervisord.conf start orchestrator workflow-runner fanout agent-runner-py
    echo "✅ Backend services started"
    sleep 2
}

view_logs() {
    echo ""
    echo "Select log to view:"
    echo "  1) Orchestrator"
    echo "  2) Workflow Runner"
    echo "  3) Fanout"
    echo "  4) Frontend"
    echo "  5) Agent Runner"
    echo "  6) All (combined)"
    echo ""
    read -p "Choice: " log_choice
    
    case $log_choice in
        1) tail -f logs/orchestrator.log ;;
        2) tail -f logs/workflow-runner.log ;;
        3) tail -f logs/fanout.log ;;
        4) tail -f logs/frontend.log ;;
        5) tail -f logs/agent-runner-py.log ;;
        6) tail -f logs/*.log ;;
        *) echo "Invalid choice" ;;
    esac
}

restart_service() {
    echo ""
    echo "Select service to restart:"
    echo "  1) Orchestrator"
    echo "  2) Workflow Runner"
    echo "  3) Fanout"
    echo "  4) Frontend"
    echo "  5) All"
    echo ""
    read -p "Choice: " restart_choice
    
    case $restart_choice in
        1) supervisorctl -c supervisord.conf restart orchestrator ;;
        2) supervisorctl -c supervisord.conf restart workflow-runner ;;
        3) supervisorctl -c supervisord.conf restart fanout ;;
        4) supervisorctl -c supervisord.conf restart frontend ;;
        5) supervisorctl -c supervisord.conf restart all ;;
        *) echo "Invalid choice" ;;
    esac
    
    echo "✅ Done"
    sleep 2
}

# Main loop
while true; do
    show_menu
    read -p "Enter choice: " choice
    
    case $choice in
        1)
            ./start.sh
            read -p "Press enter to continue..."
            ;;
        2)
            if [ ! -f pids/supervisor.sock ]; then
                ./start.sh
            fi
            start_core
            ;;
        3)
            if [ ! -f pids/supervisor.sock ]; then
                ./start.sh
            fi
            start_backend
            ;;
        4)
            if [ ! -f pids/supervisor.sock ]; then
                ./start.sh
            fi
            supervisorctl -c supervisord.conf start frontend
            echo "✅ Frontend started"
            sleep 2
            ;;
        5)
            ./stop.sh
            read -p "Press enter to continue..."
            ;;
        6)
            restart_service
            ;;
        7)
            view_logs
            ;;
        8)
            ./status.sh
            read -p "Press enter to continue..."
            ;;
        0)
            echo "Goodbye!"
            exit 0
            ;;
        *)
            echo "Invalid option"
            sleep 1
            ;;
    esac
done
