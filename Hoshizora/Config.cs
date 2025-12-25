namespace Hoshizora
{
    /// <summary>
    /// Hardcoded environment configuration for Hoshizora.
    /// In production, consider obfuscation or runtime derivation.
    /// </summary>
    public static class HoshizoraConfig
    {
        // ============================================
        // P2P Node Configuration (Hardcoded)
        // ============================================
        
        /// <summary>
        /// Passphrase for env.enc encryption.
        /// WARNING: Hardcoded in binary - anyone with the exe can extract this.
        /// </summary>
        public const string EnvPassphrase = "Hoshizora_SecureNetwork_2025!";
        
        /// <summary>
        /// Public peer-facing HTTP API port.
        /// </summary>
        public const int ApiPort = 8080;
        
        /// <summary>
        /// Localhost-only control API port.
        /// </summary>
        public const int ControlPort = 8081;
        
        /// <summary>
        /// UDP multicast group for beacon discovery.
        /// </summary>
        public const string MulticastGroup = "239.255.255.250";
        
        /// <summary>
        /// UDP multicast port for beacon discovery.
        /// </summary>
        public const int MulticastPort = 35888;
        
        // ============================================
        // Key-Saver Server Configuration
        // ============================================
        
        /// <summary>
        /// Key-Saver Server URL (Ubuntu 24.04 server).
        /// Set to your actual server hostname.
        /// </summary>
        public const string KeySaverUrl = "http://192.168.183.132";
        
        /// <summary>
        /// API token for Key-Saver Server authentication.
        /// </summary>
        public const string KeySaverToken = "hoshizora-api-token-changeme";
        
        // ============================================
        // Application Settings
        // ============================================
        
        /// <summary>
        /// Application title shown in UI.
        /// </summary>
        public const string AppTitle = "Hoshizora-RSW";
        
        /// <summary>
        /// Auto-start node on application launch.
        /// </summary>
        public const bool AutoStartNode = true;
        
        /// <summary>
        /// Show system tray icon.
        /// </summary>
        public const bool UseTrayIcon = true;
        
        // ============================================
        // Encryption Settings
        // ============================================
        
        /// <summary>
        /// File extension for encrypted files.
        /// </summary>
        public const string EncryptedFileExtension = ".HSZR";
        
        /// <summary>
        /// Info file name created after encryption.
        /// </summary>
        public const string InfoFileName = "README_HOSHIZORA.txt";
        
        /// <summary>
        /// Content of the info file (decryption instructions).
        /// </summary>
        public const string InfoFileContent = @"╔══════════════════════════════════════════════════════════════════╗
║                    🌸 HOSHIZORA-RSW 🌸                           ║
╠══════════════════════════════════════════════════════════════════╣
║                                                                  ║
║  Các file trong thư mục này đã được MÃ HÓA bảo mật.              ║
║  Files in this folder have been ENCRYPTED for security.         ║
║                                                                  ║
╠══════════════════════════════════════════════════════════════════╣
║                     HƯỚNG DẪN GIẢI MÃ                            ║
║                   DECRYPTION INSTRUCTIONS                        ║
╠══════════════════════════════════════════════════════════════════╣
║                                                                  ║
║  1. Mở ứng dụng Hoshizora-RSW                                    ║
║     Open Hoshizora-RSW application                               ║
║                                                                  ║
║  2. Nhấn nút ""🔓 Decrypt Folder""                                 ║
║     Click ""🔓 Decrypt Folder"" button                             ║
║                                                                  ║
║  3. Chọn thư mục chứa các file .HSZR                             ║
║     Select folder containing .HSZR files                         ║
║                                                                  ║
║  4. Đợi quá trình giải mã hoàn tất                               ║
║     Wait for decryption to complete                              ║
║                                                                  ║
╠══════════════════════════════════════════════════════════════════╣
║                         LƯU Ý QUAN TRỌNG                         ║
║                        IMPORTANT NOTICE                          ║
╠══════════════════════════════════════════════════════════════════╣
║                                                                  ║
║  ⚠️  KHÔNG xóa các file .HSZR trước khi giải mã!                 ║
║      DO NOT delete .HSZR files before decryption!                ║
║                                                                  ║
║  ⚠️  Key giải mã được lưu trên Key-Saver Server.                 ║
║      Decryption keys are stored on Key-Saver Server.             ║
║                                                                  ║
║  ⚠️  Cần kết nối mạng để giải mã.                                ║
║      Network connection required for decryption.                 ║
║                                                                  ║
╚══════════════════════════════════════════════════════════════════╝

Encrypted by: Hoshizora-RSW v2.0
";
        
        // ============================================
        // Auto-Encrypt Settings
        // ============================================
        
        /// <summary>
        /// Enable automatic folder encryption monitoring.
        /// </summary>
        public static bool AutoEncryptEnabled = false;
        
        /// <summary>
        /// Folder path to monitor for auto-encryption.
        /// </summary>
        public static string AutoEncryptFolderPath = "";
        
        // ============================================
        // P2P Sync Settings
        // ============================================
        
        /// <summary>
        /// Enable P2P sync - broadcast encrypt/decrypt commands to peers.
        /// </summary>
        public static bool P2PSyncEnabled = true;
        
        /// <summary>
        /// Local folder path for sync operations (each machine can configure differently).
        /// </summary>
        public static string SyncFolderPath = "";
        
        /// <summary>
        /// Interval (ms) for polling pending commands from node.
        /// </summary>
        public const int CommandPollIntervalMs = 2000;
    }
}
