using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace Hoshizora;

/// <summary>
/// Handles file encryption/decryption with Key-Saver server integration.
/// Files are encrypted with AES-256-GCM, keys are stored on the Key-Saver server.
/// </summary>
public class FileEncryption
{
    private readonly HttpClient _http;
    private readonly string _keySaverUrl;
    private readonly string _keySaverToken;
    private readonly string _nodeId;

    public event Action<string>? OnLog;

    public FileEncryption(string keySaverUrl, string keySaverToken, string nodeId)
    {
        _keySaverUrl = keySaverUrl.TrimEnd('/');
        _keySaverToken = keySaverToken;
        _nodeId = nodeId;

        _http = new HttpClient
        {
            Timeout = TimeSpan.FromSeconds(30)
        };
        _http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", _keySaverToken);
    }

    /// <summary>
    /// Encrypt all files in a folder, delete originals, upload keys to server.
    /// </summary>
    public async Task<(int encrypted, int failed)> EncryptFolderAsync(string folderPath, bool recursive, CancellationToken ct = default)
    {
        if (!Directory.Exists(folderPath))
            throw new DirectoryNotFoundException($"Folder not found: {folderPath}");

        var searchOption = recursive ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly;
        var files = Directory.GetFiles(folderPath, "*", searchOption)
            .Where(f => !f.EndsWith(".henc")) // Skip already encrypted files
            .ToList();

        int encrypted = 0, failed = 0;

        foreach (var filePath in files)
        {
            ct.ThrowIfCancellationRequested();

            try
            {
                await EncryptFileAsync(filePath);
                encrypted++;
            }
            catch (Exception ex)
            {
                Log($"[ERROR] {Path.GetFileName(filePath)}: {ex.Message}");
                failed++;
            }
        }

        return (encrypted, failed);
    }

    /// <summary>
    /// Decrypt all .henc files in a folder, fetch keys from server, restore originals.
    /// </summary>
    public async Task<(int decrypted, int failed)> DecryptFolderAsync(string folderPath, bool recursive, CancellationToken ct = default)
    {
        if (!Directory.Exists(folderPath))
            throw new DirectoryNotFoundException($"Folder not found: {folderPath}");

        var searchOption = recursive ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly;
        var files = Directory.GetFiles(folderPath, "*.henc", searchOption).ToList();

        int decrypted = 0, failed = 0;

        foreach (var filePath in files)
        {
            ct.ThrowIfCancellationRequested();

            try
            {
                await DecryptFileAsync(filePath);
                decrypted++;
            }
            catch (Exception ex)
            {
                Log($"[ERROR] {Path.GetFileName(filePath)}: {ex.Message}");
                failed++;
            }
        }

        return (decrypted, failed);
    }

    /// <summary>
    /// Encrypt a single file: generate key, encrypt, save .henc, delete original, upload key.
    /// </summary>
    private async Task EncryptFileAsync(string filePath)
    {
        string fileName = Path.GetFileName(filePath);
        Log($"Encrypting: {fileName}");

        // Generate random key and nonce
        byte[] key = RandomNumberGenerator.GetBytes(32); // AES-256
        byte[] nonce = RandomNumberGenerator.GetBytes(12); // GCM nonce

        // Read original file
        byte[] plaintext = await File.ReadAllBytesAsync(filePath);

        // Encrypt with AES-GCM
        byte[] ciphertext = new byte[plaintext.Length];
        byte[] tag = new byte[16]; // GCM tag

        using var aes = new AesGcm(key, 16);
        aes.Encrypt(nonce, plaintext, ciphertext, tag);

        // Compute hash for key identification
        string fileHash = ComputeSha256(ciphertext);

        // Save encrypted file: [12-byte nonce][16-byte tag][ciphertext]
        string encPath = filePath + ".henc";
        using (var fs = File.Create(encPath))
        {
            await fs.WriteAsync(nonce);
            await fs.WriteAsync(tag);
            await fs.WriteAsync(ciphertext);
        }

        // Upload key to Key-Saver server
        await UploadKeyAsync(fileHash, key, fileName);

        // Delete original file
        File.Delete(filePath);
        Log($"Encrypted: {fileName} → {Path.GetFileName(encPath)}");
    }

    /// <summary>
    /// Decrypt a single .henc file: fetch key from server, decrypt, restore, delete .henc.
    /// </summary>
    private async Task DecryptFileAsync(string encPath)
    {
        string fileName = Path.GetFileName(encPath);
        Log($"Decrypting: {fileName}");

        // Read encrypted file
        byte[] encData = await File.ReadAllBytesAsync(encPath);
        if (encData.Length < 28) // 12 + 16 minimum
            throw new InvalidDataException("Encrypted file too short");

        byte[] nonce = encData[..12];
        byte[] tag = encData[12..28];
        byte[] ciphertext = encData[28..];

        // Compute hash to get key
        string fileHash = ComputeSha256(ciphertext);

        // Fetch key from server
        byte[] key = await FetchKeyAsync(fileHash);

        // Decrypt
        byte[] plaintext = new byte[ciphertext.Length];
        using var aes = new AesGcm(key, 16);
        aes.Decrypt(nonce, ciphertext, tag, plaintext);

        // Restore original file (remove .henc extension)
        string originalPath = encPath[..^5]; // Remove ".henc"
        await File.WriteAllBytesAsync(originalPath, plaintext);

        // Delete encrypted file
        File.Delete(encPath);
        Log($"Decrypted: {fileName} → {Path.GetFileName(originalPath)}");
    }

    private async Task UploadKeyAsync(string fileHash, byte[] key, string fileName)
    {
        var payload = new
        {
            hash = fileHash,
            key_b64 = Convert.ToBase64String(key),
            node_id = _nodeId,
            name = fileName
        };

        var content = new StringContent(JsonSerializer.Serialize(payload), Encoding.UTF8, "application/json");
        var response = await _http.PostAsync($"{_keySaverUrl}/keys/save", content);

        if (!response.IsSuccessStatusCode)
        {
            string error = await response.Content.ReadAsStringAsync();
            throw new HttpRequestException($"Key upload failed: {error}");
        }

        Log($"Key uploaded: {fileHash[..16]}...");
    }

    private async Task<byte[]> FetchKeyAsync(string fileHash)
    {
        var response = await _http.GetAsync($"{_keySaverUrl}/keys/get?hash={fileHash}");

        if (!response.IsSuccessStatusCode)
        {
            string error = await response.Content.ReadAsStringAsync();
            throw new HttpRequestException($"Key fetch failed: {error}");
        }

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);

        if (!doc.RootElement.TryGetProperty("key_b64", out var keyEl))
            throw new KeyNotFoundException($"Key not found for hash: {fileHash}");

        string keyB64 = keyEl.GetString() ?? throw new InvalidDataException("Empty key");
        return Convert.FromBase64String(keyB64);
    }

    private static string ComputeSha256(byte[] data)
    {
        byte[] hash = SHA256.HashData(data);
        return Convert.ToHexString(hash).ToLowerInvariant();
    }

    private void Log(string msg) => OnLog?.Invoke(msg);
}
