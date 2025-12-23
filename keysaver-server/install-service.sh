#!/bin/bash
# Install keysaver-server as a systemd service on Ubuntu 24.04
# Run as root: sudo ./install-service.sh

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

INSTALL_DIR="/opt/keysaver"
SERVICE_USER="keysaver"
SERVICE_NAME="keysaver"

echo -e "${GREEN}=== Hoshizora Key-Saver Server Installation ===${NC}"

# Check root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root (sudo ./install-service.sh)${NC}"
    exit 1
fi

# Check if binary exists
if [ ! -f "keysaver-server" ]; then
    echo -e "${YELLOW}Building keysaver-server...${NC}"
    go build -o keysaver-server .
fi

# Create service user
if ! id "$SERVICE_USER" &>/dev/null; then
    echo "Creating user: $SERVICE_USER"
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
fi

# Create directories
echo "Creating directories..."
mkdir -p "$INSTALL_DIR/data"
mkdir -p "$INSTALL_DIR/certs"

# Copy binary
echo "Installing binary..."
cp keysaver-server "$INSTALL_DIR/"
chmod 755 "$INSTALL_DIR/keysaver-server"

# Create environment file if not exists
if [ ! -f "$INSTALL_DIR/.env" ]; then
    echo "Creating environment file..."
    cat > "$INSTALL_DIR/.env" << 'EOF'
# Key-Saver Server Configuration
# Edit this file and restart the service

# Master key for encrypting stored keys (REQUIRED - change this!)
MASTER_KEY=CHANGE_THIS_TO_A_SECURE_RANDOM_STRING

# API tokens (comma-separated, optional for authentication)
# KEYSAVER_TOKENS=token1,token2,token3
EOF
    chmod 600 "$INSTALL_DIR/.env"
fi

# Set ownership
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

# Install systemd service
echo "Installing systemd service..."
cp keysaver.service /etc/systemd/system/
systemctl daemon-reload

echo ""
echo -e "${GREEN}=== Installation Complete ===${NC}"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Edit the environment file with your master key:"
echo "   sudo nano $INSTALL_DIR/.env"
echo ""
echo "2. Add TLS certificates:"
echo "   sudo cp your-cert.crt $INSTALL_DIR/certs/server.crt"
echo "   sudo cp your-cert.key $INSTALL_DIR/certs/server.key"
echo "   sudo chown $SERVICE_USER:$SERVICE_USER $INSTALL_DIR/certs/*"
echo ""
echo "3. Enable and start the service:"
echo "   sudo systemctl enable $SERVICE_NAME"
echo "   sudo systemctl start $SERVICE_NAME"
echo ""
echo "4. Check status:"
echo "   sudo systemctl status $SERVICE_NAME"
echo "   sudo journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${YELLOW}For development (HTTP mode without TLS):${NC}"
echo "   Modify /etc/systemd/system/keysaver.service"
echo "   Add --http flag and remove --cert/--key flags"
