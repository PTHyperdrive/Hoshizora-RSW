# 🌸 Hoshizora-RSW

> **Version:** `v2.0.0`  
> **Status:** Stable — Key-Saver Server, Hoshizora Client, DLL exports  
> **New in v2:** Centralized key storage, C# client with folder encryption

---

## 🔍 Overview

**Hoshizora-RSW** is a peer-to-peer storage layer for secure, resilient, and private file distribution.  
Each node acts as a **mini data vault** (up to 1 GB) and participates in an **encrypted mesh network** built on mixnet principles.

### Core Features
| Feature | Description |
|----------|--------------|
| **Automatic Node Discovery** | Nodes announce via encrypted UDP beacons |
| **Encrypted Communication** | XChaCha20-Poly1305 using `env.enc` keys |
| **Key-Saver Server** | Secure remote key storage (Ubuntu 24.04) |
| **Hoshizora Client** | Windows GUI with folder encryption |
| **Blockchain-style Storage** | Encrypted chunks replicated across peers |
| **DLL Mode** | Load p2pnode as shared library via CGO |

---

## Project Structure

```
Hoshizora-RSW/
├── go-node/           # Core P2P node (Go)
├── keysaver-server/   # Key storage server (Go, Ubuntu)
├── Hoshizora/         # Windows client (C# WinForms)
└── README.md
```

---

## 🚀 Quick Start

### Option 1: Standalone Node (Go)

```bash
cd go-node
go build -o p2pnode .
export MIXNETS_ENV_PASS="YourPassphrase"
./p2pnode --new-net
```

### Option 2: Hoshizora Client (Windows)

```powershell
cd Hoshizora
dotnet build
.\bin\Debug\net8.0-windows\Hoshizora.exe
```

### Option 3: Key-Saver Server (Ubuntu 24.04)

```bash
cd keysaver-server
go build -o keysaver-server .
sudo ./install-service.sh
```

---

## Key-Saver Server

Centralized, encrypted key storage for the decentralized network.

### API Endpoints
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/keys/save` | POST | Upload encrypted key |
| `/keys/get?hash=X` | GET | Retrieve key by file hash |
| `/keys/list?node_id=X` | GET | List keys for a node |
| `/keys/delete?hash=X` | DELETE | Remove a key |
| `/health` | GET | Health check |

### Installation (Ubuntu)
```bash
cd keysaver-server
go build -o keysaver-server .
sudo chmod +x install-service.sh
sudo ./install-service.sh
sudo nano /opt/keysaver/.env  # Set MASTER_KEY
sudo systemctl enable keysaver
sudo systemctl start keysaver
```

---

## 🌸 Hoshizora Client

Windows GUI application with hardcoded environment configuration.

### Features
- **Encrypt Folder**: Select folder → AES-256-GCM encrypt → Delete originals → Upload keys
- **Decrypt Folder**: Fetch keys from server → Decrypt → Restore files
- **Dual Mode**: DLL (P/Invoke) or Subprocess (HTTP API) fallback
- **System Tray**: Minimize to tray, background operation

### Configuration (`Config.cs`)
```csharp
public const string EnvPassphrase = "Hoshizora_SecureNetwork_2025!";
public const int ApiPort = 8080;
public const int ControlPort = 8081;
public const string KeySaverUrl = "https://keys.example.com";
```

---

## 🔧 Building DLL (Windows)

Build p2pnode as a shared library for C#/.NET integration:

```powershell
cd go-node
.\build-dll.ps1 -AddExclusion  # Add Windows Defender exclusion (Admin)
.\build-dll.ps1                 # Build p2pnode.dll
```

**Exported Functions:** `P2P_Init`, `P2P_Start`, `P2P_Stop`, `P2P_GetStatus`, `P2P_GetPeers`, `P2P_FreeString`

---

## Local Control API (`localhost:8081`)

### Status & Peers
```bash
curl http://127.0.0.1:8081/status
curl http://127.0.0.1:8081/peers
```

### Send Encrypted File
```bash
curl -X POST -F "file=@report.txt" "http://127.0.0.1:8081/mix/send-file?name=report.txt"
```

### Decrypt Chunk
```bash
curl "http://127.0.0.1:8081/chunks/decrypt?hash=<sha256>&name=report.txt&out=restored.txt"
```

---

## ⚙️ Command Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--api-port` | `8080` | Peer-to-peer HTTP port |
| `--control-port` | `8081` | Localhost control port |
| `--mc-group` | `239.255.255.250` | Beacon multicast group |
| `--mc-port` | `35888` | UDP multicast port |
| `--new-net` | `false` | Generate new `env.enc` |
| `--env-pass` | *(env var)* | Passphrase for `env.enc` |

---

## 🏗️ Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Hoshizora     │     │    p2pnode      │     │  Key-Saver      │
│   (Windows)     │────▶│    (Go)         │────▶│  Server         │
│   C# WinForms   │     │  DLL/Standalone │     │  (Ubuntu)       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │                       │
        │   AES-256-GCM         │   XChaCha20-Poly1305  │   SQLite + 
        │   Folder Encrypt      │   Beacon/File Crypto  │   Encrypted Keys
        └───────────────────────┴───────────────────────┘
```

**Encryption:**
- 🔐 **Beacon:** XChaCha20-Poly1305 via `BeaconKey`
- 🗝️ **Files:** XChaCha20-Poly1305 per-file random key
- 📁 **Folders:** AES-256-GCM (Hoshizora client)
- 💾 **Key Storage:** XChaCha20-Poly1305 at rest (Key-Saver)

---

## 🧭 Roadmap
- ✅ Node Discovery, Encrypted Replication
- ✅ Key-Saver Server with TLS
- ✅ Hoshizora Windows Client
- ✅ Folder Encryption/Decryption
- 🔜 DHT-based File Index
- 🔜 STUN/TURN Discovery
- 🔜 Mobile Client (Android/iOS)

---

## ⚖️ License
MIT License © 2025  
Use, modify, and distribute freely with attribution.
