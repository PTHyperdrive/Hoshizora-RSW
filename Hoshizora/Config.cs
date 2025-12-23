namespace Hoshizora;

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
    public const string KeySaverUrl = "https://keys.example.com";
    
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
}
