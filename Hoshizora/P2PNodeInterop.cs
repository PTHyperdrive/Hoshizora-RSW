using System.Runtime.InteropServices;

namespace Hoshizora;

/// <summary>
/// P/Invoke bindings for p2pnode.dll (Go shared library).
/// All functions follow C calling convention.
/// String returns must be freed with P2P_FreeString().
/// </summary>
public static class P2PNode
{
    private const string DllName = "p2pnode.dll";

    /// <summary>
    /// Initialize the P2P node with configuration.
    /// </summary>
    /// <param name="envPass">Passphrase for env.enc encryption</param>
    /// <param name="apiPort">Public peer-facing HTTP port</param>
    /// <param name="controlPort">Localhost-only control port</param>
    /// <param name="mcGroup">Multicast group for discovery</param>
    /// <param name="mcPort">Multicast UDP port</param>
    /// <param name="keySaverUrl">Key-Saver Server URL (optional)</param>
    /// <param name="forceNewEnv">1 to recreate env.enc with new passphrase, 0 to use existing</param>
    /// <returns>0 on success, negative error code on failure</returns>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl, CharSet = CharSet.Ansi)]
    public static extern int P2P_Init(
        [MarshalAs(UnmanagedType.LPUTF8Str)] string envPass,
        int apiPort,
        int controlPort,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string mcGroup,
        int mcPort,
        [MarshalAs(UnmanagedType.LPUTF8Str)] string? keySaverUrl,
        int forceNewEnv);

    /// <summary>
    /// Start the P2P node services (discovery, HTTP servers).
    /// </summary>
    /// <returns>0 on success, negative error code on failure</returns>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern int P2P_Start();

    /// <summary>
    /// Stop all P2P services gracefully.
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern void P2P_Stop();

    /// <summary>
    /// Get JSON status of the node.
    /// Caller must free the returned string with P2P_FreeString().
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern IntPtr P2P_GetStatus();

    /// <summary>
    /// Get JSON array of discovered peers.
    /// Caller must free the returned string with P2P_FreeString().
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern IntPtr P2P_GetPeers();

    /// <summary>
    /// Get the node's unique ID.
    /// Caller must free the returned string with P2P_FreeString().
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern IntPtr P2P_GetNodeID();

    /// <summary>
    /// Get the node's public key (base64 encoded).
    /// Caller must free the returned string with P2P_FreeString().
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern IntPtr P2P_GetPublicKey();

    /// <summary>
    /// Check if the node is currently running.
    /// </summary>
    /// <returns>1 if running, 0 if stopped</returns>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern int P2P_IsRunning();

    /// <summary>
    /// Free a string returned by other P2P_* functions.
    /// </summary>
    [DllImport(DllName, CallingConvention = CallingConvention.Cdecl)]
    public static extern void P2P_FreeString(IntPtr s);

    // ========================================
    // Helper methods for safer usage
    // ========================================

    /// <summary>
    /// Get status as a managed string (handles memory management).
    /// </summary>
    public static string GetStatus()
    {
        IntPtr ptr = P2P_GetStatus();
        if (ptr == IntPtr.Zero) return "{}";
        try
        {
            return Marshal.PtrToStringUTF8(ptr) ?? "{}";
        }
        finally
        {
            P2P_FreeString(ptr);
        }
    }

    /// <summary>
    /// Get peers as a managed string (handles memory management).
    /// </summary>
    public static string GetPeers()
    {
        IntPtr ptr = P2P_GetPeers();
        if (ptr == IntPtr.Zero) return "[]";
        try
        {
            return Marshal.PtrToStringUTF8(ptr) ?? "[]";
        }
        finally
        {
            P2P_FreeString(ptr);
        }
    }

    /// <summary>
    /// Get node ID as a managed string (handles memory management).
    /// </summary>
    public static string GetNodeID()
    {
        IntPtr ptr = P2P_GetNodeID();
        if (ptr == IntPtr.Zero) return "";
        try
        {
            return Marshal.PtrToStringUTF8(ptr) ?? "";
        }
        finally
        {
            P2P_FreeString(ptr);
        }
    }

    /// <summary>
    /// Get public key as a managed string (handles memory management).
    /// </summary>
    public static string GetPublicKey()
    {
        IntPtr ptr = P2P_GetPublicKey();
        if (ptr == IntPtr.Zero) return "";
        try
        {
            return Marshal.PtrToStringUTF8(ptr) ?? "";
        }
        finally
        {
            P2P_FreeString(ptr);
        }
    }

    /// <summary>
    /// Check if node is running.
    /// </summary>
    public static bool IsRunning => P2P_IsRunning() == 1;
}
