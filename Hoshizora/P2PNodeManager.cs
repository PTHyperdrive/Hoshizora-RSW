using System.Diagnostics;
using System.Text;
using System.Text.Json;

namespace Hoshizora;

/// <summary>
/// Manages the p2pnode.exe subprocess and communicates via HTTP Control API.
/// This approach avoids DLL/CGO antivirus false positives.
/// </summary>
public class P2PNodeManager : IDisposable
{
    private Process? _nodeProcess;
    private readonly HttpClient _http;
    private bool _disposed;

    public bool IsRunning => _nodeProcess != null && !_nodeProcess.HasExited;
    public string ControlUrl { get; }

    public event Action<string>? OnLog;

    public P2PNodeManager()
    {
        ControlUrl = $"http://127.0.0.1:{HoshizoraConfig.ControlPort}/";
        _http = new HttpClient
        {
            BaseAddress = new Uri(ControlUrl),
            Timeout = TimeSpan.FromSeconds(10)
        };
    }

    /// <summary>
    /// Start the p2pnode.exe subprocess with hardcoded configuration.
    /// </summary>
    public async Task<bool> StartAsync()
    {
        if (IsRunning)
        {
            Log("Node already running");
            return true;
        }

        // Find p2pnode.exe
        string exePath = FindNodeExecutable();
        if (string.IsNullOrEmpty(exePath))
        {
            Log("[ERROR] p2pnode.exe not found");
            return false;
        }

        Log($"Starting: {exePath}");

        // Build arguments with hardcoded config
        var args = new StringBuilder();
        args.Append($"--api-port {HoshizoraConfig.ApiPort} ");
        args.Append($"--control-port {HoshizoraConfig.ControlPort} ");
        args.Append($"--mc-group {HoshizoraConfig.MulticastGroup} ");
        args.Append($"--mc-port {HoshizoraConfig.MulticastPort} ");

        // Set environment variable for passphrase
        var startInfo = new ProcessStartInfo
        {
            FileName = exePath,
            Arguments = args.ToString().Trim(),
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = Path.GetDirectoryName(exePath)
        };

        // Set passphrase via environment variable (secure - not in command line)
        startInfo.EnvironmentVariables["MIXNETS_ENV_PASS"] = HoshizoraConfig.EnvPassphrase;

        try
        {
            _nodeProcess = Process.Start(startInfo);
            if (_nodeProcess == null)
            {
                Log("[ERROR] Failed to start process");
                return false;
            }

            // Capture output
            _nodeProcess.OutputDataReceived += (s, e) =>
            {
                if (!string.IsNullOrEmpty(e.Data)) Log($"[node] {e.Data}");
            };
            _nodeProcess.ErrorDataReceived += (s, e) =>
            {
                if (!string.IsNullOrEmpty(e.Data)) Log($"[node] {e.Data}");
            };
            _nodeProcess.BeginOutputReadLine();
            _nodeProcess.BeginErrorReadLine();

            Log($"Process started (PID: {_nodeProcess.Id})");

            // Wait for API to become available
            bool ready = await WaitForApiReady(TimeSpan.FromSeconds(15));
            if (!ready)
            {
                Log("[WARNING] API not responding, but process is running");
            }
            else
            {
                Log("Control API ready");
            }

            return true;
        }
        catch (Exception ex)
        {
            Log($"[ERROR] Start failed: {ex.Message}");
            return false;
        }
    }

    /// <summary>
    /// Stop the p2pnode.exe subprocess gracefully.
    /// </summary>
    public void Stop()
    {
        if (_nodeProcess == null) return;

        try
        {
            if (!_nodeProcess.HasExited)
            {
                Log("Stopping node process...");
                _nodeProcess.Kill(entireProcessTree: true);
                _nodeProcess.WaitForExit(3000);
            }
            Log("Node stopped");
        }
        catch (Exception ex)
        {
            Log($"[ERROR] Stop failed: {ex.Message}");
        }
        finally
        {
            _nodeProcess?.Dispose();
            _nodeProcess = null;
        }
    }

    /// <summary>
    /// Get node status from Control API.
    /// </summary>
    public async Task<JsonElement?> GetStatusAsync()
    {
        try
        {
            var response = await _http.GetStringAsync("status");
            return JsonDocument.Parse(response).RootElement.Clone();
        }
        catch
        {
            return null;
        }
    }

    /// <summary>
    /// Get peers list from Control API.
    /// </summary>
    public async Task<JsonElement?> GetPeersAsync()
    {
        try
        {
            var response = await _http.GetStringAsync("peers");
            return JsonDocument.Parse(response).RootElement.Clone();
        }
        catch
        {
            return null;
        }
    }

    /// <summary>
    /// Get sync status from Control API.
    /// </summary>
    public async Task<JsonElement?> GetSyncStatusAsync()
    {
        try
        {
            var response = await _http.GetStringAsync("sync/status");
            return JsonDocument.Parse(response).RootElement.Clone();
        }
        catch
        {
            return null;
        }
    }

    /// <summary>
    /// Send a file for encrypted distribution.
    /// </summary>
    public async Task<JsonElement?> SendFileAsync(string filePath)
    {
        if (!File.Exists(filePath))
            throw new FileNotFoundException("File not found", filePath);

        var fileName = Path.GetFileName(filePath);
        var fileBytes = await File.ReadAllBytesAsync(filePath);

        var content = new ByteArrayContent(fileBytes);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/octet-stream");

        var response = await _http.PostAsync($"mix/send-file?name={Uri.EscapeDataString(fileName)}", content);
        var responseText = await response.Content.ReadAsStringAsync();

        if (response.IsSuccessStatusCode)
        {
            return JsonDocument.Parse(responseText).RootElement.Clone();
        }
        else
        {
            throw new HttpRequestException($"Send file failed: {responseText}");
        }
    }

    private async Task<bool> WaitForApiReady(TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        while (DateTime.UtcNow < deadline)
        {
            try
            {
                var response = await _http.GetAsync("status");
                if (response.IsSuccessStatusCode)
                    return true;
            }
            catch { }

            await Task.Delay(500);
        }
        return false;
    }

    private string FindNodeExecutable()
    {
        // Search in common locations
        var searchPaths = new[]
        {
            Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "p2pnode.exe"),
            Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "..", "go-node", "p2pnode.exe"),
            Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "..", "..", "go-node", "p2pnode.exe"),
            Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "..", "..", "..", "go-node", "p2pnode.exe"),
            @"e:\Codes\Decentralize-DataNodes\go-node\p2pnode.exe", // Fallback absolute path
        };

        foreach (var path in searchPaths)
        {
            var fullPath = Path.GetFullPath(path);
            if (File.Exists(fullPath))
                return fullPath;
        }

        return string.Empty;
    }

    private void Log(string message) => OnLog?.Invoke(message);

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;

        Stop();
        _http.Dispose();
    }
}
