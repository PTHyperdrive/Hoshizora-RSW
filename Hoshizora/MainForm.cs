using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace Hoshizora
{
    public class MainForm : Form
    {
        private Label lblStatus;
        private Label lblNodeId;
        private Label lblPeersCount;
        private Label lblSyncStatus;
        private Label lblMode;
        private Button btnStartStop;
        private Button btnRefresh;
        private DataGridView gridPeers;
        private TextBox txtLog;
        private System.Windows.Forms.Timer refreshTimer;
        private NotifyIcon trayIcon;

        // Two modes: DLL (preferred) or Subprocess (fallback)
        private bool _useDllMode = true;
        private bool _dllLoaded = false;
        private bool _nodeInitialized = false;
        private P2PNodeManager _subprocessManager;

        // Encryption UI
        private Button btnEncrypt;
        private Button btnDecrypt;
        private CheckBox chkRecursive;
        private Label lblFolder;
        private string _selectedFolder;
        
        // Auto-encrypt
        private CheckBox chkAutoEncrypt;
        private Button btnSelectAutoFolder;
        private Label lblAutoFolder;
        private FileSystemWatcher _autoEncryptWatcher;

        public MainForm()
        {
            Text = HoshizoraConfig.AppTitle;
            Width = 900;
            Height = 680;
            StartPosition = FormStartPosition.CenterScreen;
            Icon = SystemIcons.Application;

            InitializeLayout();
            InitializeTimer();
            InitializeP2PSync();

            if (HoshizoraConfig.UseTrayIcon)
            {
                InitializeTrayIcon();
            }

            Load += MainForm_Load;
            FormClosing += MainForm_FormClosing;
        }

        private void InitializeLayout()
        {
            var root = new TableLayoutPanel
            {
                Dock = DockStyle.Fill,
                RowCount = 6,
                ColumnCount = 1,
                Padding = new Padding(10)
            };
            root.RowStyles.Add(new RowStyle(SizeType.AutoSize)); // Row 0: Status
            root.RowStyles.Add(new RowStyle(SizeType.AutoSize)); // Row 1: Selected folder
            root.RowStyles.Add(new RowStyle(SizeType.AutoSize)); // Row 2: Auto-encrypt
            root.RowStyles.Add(new RowStyle(SizeType.Percent, 40)); // Row 3: Peers grid
            root.RowStyles.Add(new RowStyle(SizeType.Percent, 25)); // Row 4: Config
            root.RowStyles.Add(new RowStyle(SizeType.Percent, 35)); // Row 5: Log
            Controls.Add(root);

            // Status panel
            var pnlStatus = new FlowLayoutPanel
            {
                Dock = DockStyle.Fill,
                AutoSize = true,
                WrapContents = true
            };

            lblStatus = new Label
            {
                Text = "● Not Started",
                AutoSize = true,
                Font = new Font(Font.FontFamily, 10, FontStyle.Bold),
                ForeColor = Color.DarkRed,
                Margin = new Padding(0, 3, 15, 3)
            };

            lblMode = new Label
            {
                Text = "[DLL]",
                AutoSize = true,
                Font = new Font(Font.FontFamily, 8),
                ForeColor = Color.Gray,
                Margin = new Padding(0, 5, 15, 3)
            };

            lblNodeId = new Label
            {
                Text = "Node: -",
                AutoSize = true,
                Margin = new Padding(0, 3, 15, 3)
            };

            lblPeersCount = new Label
            {
                Text = "Peers: 0",
                AutoSize = true,
                ForeColor = Color.DarkBlue,
                Margin = new Padding(0, 3, 15, 3)
            };

            lblSyncStatus = new Label
            {
                Text = "Blocks: - | Chunks: -",
                AutoSize = true,
                ForeColor = Color.DarkGreen,
                Margin = new Padding(0, 3, 15, 3)
            };

            btnStartStop = new Button
            {
                Text = "▶ Start Node",
                Width = 110,
                Height = 28,
                BackColor = Color.FromArgb(40, 167, 69),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat,
                Cursor = Cursors.Hand,
                Margin = new Padding(10, 0, 5, 0)
            };
            btnStartStop.FlatAppearance.BorderSize = 0;
            btnStartStop.Click += BtnStartStop_Click;

            btnRefresh = new Button
            {
                Text = "⟳ Refresh",
                Width = 80,
                Height = 28,
                Margin = new Padding(5, 0, 0, 0)
            };
            btnRefresh.Click += async (s, e) => await RefreshStatusAsync();

            // Encryption buttons
            btnEncrypt = new Button
            {
                Text = "🔒 Encrypt Folder",
                Width = 120,
                Height = 28,
                BackColor = Color.FromArgb(0, 123, 255),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat,
                Cursor = Cursors.Hand,
                Margin = new Padding(15, 0, 5, 0)
            };
            btnEncrypt.FlatAppearance.BorderSize = 0;
            btnEncrypt.Click += BtnEncrypt_Click;

            btnDecrypt = new Button
            {
                Text = "🔓 Decrypt Folder",
                Width = 120,
                Height = 28,
                BackColor = Color.FromArgb(108, 117, 125),
                ForeColor = Color.White,
                FlatStyle = FlatStyle.Flat,
                Cursor = Cursors.Hand,
                Margin = new Padding(5, 0, 5, 0)
            };
            btnDecrypt.FlatAppearance.BorderSize = 0;
            btnDecrypt.Click += BtnDecrypt_Click;

            chkRecursive = new CheckBox
            {
                Text = "Recursive",
                AutoSize = true,
                Checked = true,
                Margin = new Padding(10, 5, 0, 0)
            };

            pnlStatus.Controls.AddRange(new Control[] { lblStatus, lblMode, lblNodeId, lblPeersCount, lblSyncStatus, btnStartStop, btnRefresh, btnEncrypt, btnDecrypt, chkRecursive });
            root.Controls.Add(pnlStatus, 0, 0);

            // Selected folder
            var pnlFolder = new FlowLayoutPanel
            {
                Dock = DockStyle.Fill,
                AutoSize = true,
                WrapContents = false
            };

            var lblFolderTitle = new Label
            {
                Text = "Selected Folder: ",
                AutoSize = true,
                Margin = new Padding(0, 3, 0, 3)
            };

            lblFolder = new Label
            {
                Text = "(None - use Encrypt/Decrypt buttons to select)",
                AutoSize = true,
                ForeColor = Color.Gray,
                Margin = new Padding(0, 3, 0, 3)
            };

            pnlFolder.Controls.Add(lblFolderTitle);
            pnlFolder.Controls.Add(lblFolder);
            root.Controls.Add(pnlFolder, 0, 1);

            // Auto-encrypt panel
            var pnlAutoEncrypt = new FlowLayoutPanel
            {
                Dock = DockStyle.Fill,
                AutoSize = true,
                WrapContents = false
            };

            chkAutoEncrypt = new CheckBox
            {
                Text = "🔄 Auto-Encrypt Folder:",
                AutoSize = true,
                Checked = HoshizoraConfig.AutoEncryptEnabled,
                Margin = new Padding(0, 3, 5, 3)
            };
            chkAutoEncrypt.CheckedChanged += ChkAutoEncrypt_CheckedChanged;

            btnSelectAutoFolder = new Button
            {
                Text = "📁 Select",
                Width = 70,
                Height = 24,
                Margin = new Padding(0, 0, 5, 0)
            };
            btnSelectAutoFolder.Click += BtnSelectAutoFolder_Click;

            lblAutoFolder = new Label
            {
                Text = string.IsNullOrEmpty(HoshizoraConfig.AutoEncryptFolderPath) 
                    ? "(No folder selected)" 
                    : HoshizoraConfig.AutoEncryptFolderPath,
                AutoSize = true,
                ForeColor = Color.Gray,
                Margin = new Padding(0, 5, 0, 3)
            };

            pnlAutoEncrypt.Controls.Add(chkAutoEncrypt);
            pnlAutoEncrypt.Controls.Add(btnSelectAutoFolder);
            pnlAutoEncrypt.Controls.Add(lblAutoFolder);
            root.Controls.Add(pnlAutoEncrypt, 0, 2);

            // Peers grid
            var pnlPeers = new GroupBox
            {
                Text = "Discovered Peers",
                Dock = DockStyle.Fill
            };

            gridPeers = new DataGridView
            {
                Dock = DockStyle.Fill,
                ReadOnly = true,
                AutoSizeColumnsMode = DataGridViewAutoSizeColumnsMode.Fill,
                AllowUserToAddRows = false,
                AllowUserToDeleteRows = false,
                SelectionMode = DataGridViewSelectionMode.FullRowSelect,
                BackgroundColor = SystemColors.Window,
                BorderStyle = BorderStyle.None
            };
            gridPeers.Columns.Add("NodeID", "Node ID");
            gridPeers.Columns.Add("Address", "Address");
            gridPeers.Columns.Add("Hostname", "Hostname");
            gridPeers.Columns.Add("LastSeen", "Last Seen");
            gridPeers.CellDoubleClick += GridPeers_CellDoubleClick;

            pnlPeers.Controls.Add(gridPeers);
            root.Controls.Add(pnlPeers, 0, 3);

            // Config info
            var pnlConfig = new GroupBox
            {
                Text = "Configuration (Hardcoded)",
                Dock = DockStyle.Fill
            };

            var configInfo = new TextBox
            {
                Dock = DockStyle.Fill,
                Multiline = true,
                ReadOnly = true,
                BorderStyle = BorderStyle.None,
                BackColor = SystemColors.Control,
                Font = new Font("Consolas", 9),
                Text = string.Format(
                    "API Port: {0}  |  Control Port: {1}\r\n" +
                    "Multicast: {2}:{3}\r\n" +
                    "Key-Saver: {4}",
                    HoshizoraConfig.ApiPort, HoshizoraConfig.ControlPort,
                    HoshizoraConfig.MulticastGroup, HoshizoraConfig.MulticastPort,
                    HoshizoraConfig.KeySaverUrl)
            };
            pnlConfig.Controls.Add(configInfo);
            root.Controls.Add(pnlConfig, 0, 4);

            // Log
            var pnlLog = new GroupBox
            {
                Text = "Log",
                Dock = DockStyle.Fill
            };

            txtLog = new TextBox
            {
                Dock = DockStyle.Fill,
                Multiline = true,
                ReadOnly = true,
                ScrollBars = ScrollBars.Vertical,
                Font = new Font("Consolas", 9),
                BackColor = Color.FromArgb(30, 30, 30),
                ForeColor = Color.LightGray
            };
            pnlLog.Controls.Add(txtLog);
            root.Controls.Add(pnlLog, 0, 5);
        }

        private void InitializeTimer()
        {
            refreshTimer = new System.Windows.Forms.Timer { Interval = 5000 };
            refreshTimer.Tick += async (s, e) => await RefreshStatusAsync();
        }

        private void InitializeTrayIcon()
        {
            trayIcon = new NotifyIcon
            {
                Text = HoshizoraConfig.AppTitle,
                Visible = true,
                Icon = SystemIcons.Application
            };

            var menu = new ContextMenuStrip();
            menu.Items.Add("Show", null, (s, e) => { Show(); WindowState = FormWindowState.Normal; });
            menu.Items.Add("Start/Stop", null, (s, e) => BtnStartStop_Click(s, e));
            menu.Items.Add(new ToolStripSeparator());
            menu.Items.Add("Exit", null, (s, e) => Application.Exit());

            trayIcon.ContextMenuStrip = menu;
            trayIcon.DoubleClick += (s, e) => { Show(); WindowState = FormWindowState.Normal; };
        }

        private async void MainForm_Load(object sender, EventArgs e)
        {
            Log("Hoshizora starting...");

            // Try DLL first
            string dllPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "p2pnode.dll");
            if (File.Exists(dllPath))
            {
                try
                {
                    // Test if DLL can be loaded
                    var _ = P2PNode.P2P_IsRunning();
                    _dllLoaded = true;
                    _useDllMode = true;
                    lblMode.Text = "[DLL Mode]";
                    lblMode.ForeColor = Color.DarkGreen;
                    Log("p2pnode.dll loaded successfully");
                }
                catch (DllNotFoundException)
                {
                    Log("[WARNING] p2pnode.dll found but failed to load - using subprocess mode");
                    _useDllMode = false;
                }
                catch (Exception ex)
                {
                    Log(string.Format("[WARNING] DLL load error: {0} - using subprocess mode", ex.Message));
                    _useDllMode = false;
                }
            }
            else
            {
                Log("p2pnode.dll not found - using subprocess mode");
                _useDllMode = false;
            }

            // Setup subprocess manager as fallback
            if (!_useDllMode)
            {
                lblMode.Text = "[Subprocess Mode]";
                lblMode.ForeColor = Color.DarkOrange;
                _subprocessManager = new P2PNodeManager();
                _subprocessManager.OnLog += Log;
            }

            if (HoshizoraConfig.AutoStartNode)
            {
                await Task.Delay(300);
                BtnStartStop_Click(this, EventArgs.Empty);
            }
        }

        private void MainForm_FormClosing(object sender, FormClosingEventArgs e)
        {
            refreshTimer.Stop();
            StopAutoEncryptWatcher();

            if (_useDllMode && _nodeInitialized)
            {
                try { P2PNode.P2P_Stop(); } catch { }
            }

            if (_subprocessManager != null)
                _subprocessManager.Dispose();
            if (trayIcon != null)
                trayIcon.Dispose();
        }

        private async void BtnStartStop_Click(object sender, EventArgs e)
        {
            btnStartStop.Enabled = false;

            try
            {
                if (_useDllMode)
                    await HandleDllModeToggle();
                else
                    await HandleSubprocessModeToggle();
            }
            catch (Exception ex)
            {
                Log(string.Format("[ERROR] {0}", ex.Message));
                MessageBox.Show(ex.Message, "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
            }
            finally
            {
                btnStartStop.Enabled = true;
            }
        }

        private async Task HandleDllModeToggle()
        {
            bool isRunning = _nodeInitialized && P2PNode.IsRunning;

            if (isRunning)
            {
                Log("Stopping node (DLL)...");
                P2PNode.P2P_Stop();
                refreshTimer.Stop();
                SetStoppedUI();
                Log("Node stopped");
            }
            else
            {
                if (!_nodeInitialized)
                {
                    Log("Initializing node (DLL)...");
                    int result = P2PNode.P2P_Init(
                        HoshizoraConfig.EnvPassphrase,
                        HoshizoraConfig.ApiPort,
                        HoshizoraConfig.ControlPort,
                        HoshizoraConfig.MulticastGroup,
                        HoshizoraConfig.MulticastPort,
                        HoshizoraConfig.KeySaverUrl,
                        0); // 0 = use existing env if decrypts OK, create new only if fails

                    if (result != 0)
                    {
                        Log(string.Format("[ERROR] Init failed (code: {0})", result));
                        MessageBox.Show(string.Format("Node init failed (code: {0})", result), "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                        return;
                    }
                    _nodeInitialized = true;
                    Log("Node initialized");
                }

                Log("Starting node (DLL)...");
                int startResult = P2PNode.P2P_Start();
                if (startResult != 0)
                {
                    Log(string.Format("[ERROR] Start failed (code: {0})", startResult));
                    MessageBox.Show(string.Format("Node start failed (code: {0})", startResult), "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                    return;
                }

                SetRunningUI();
                refreshTimer.Start();
                await RefreshStatusAsync();
                Log("Node started successfully");
            }
        }

        private async Task HandleSubprocessModeToggle()
        {
            if (_subprocessManager == null) return;

            if (_subprocessManager.IsRunning)
            {
                _subprocessManager.Stop();
                refreshTimer.Stop();
                SetStoppedUI();
            }
            else
            {
                lblStatus.Text = "● Starting...";
                lblStatus.ForeColor = Color.Orange;

                bool started = await _subprocessManager.StartAsync();
                if (started)
                {
                    SetRunningUI();
                    refreshTimer.Start();
                    await RefreshStatusAsync();
                }
                else
                {
                    SetStoppedUI();
                    MessageBox.Show("Failed to start node. Check log.", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                }
            }
        }

        private void SetRunningUI()
        {
            lblStatus.Text = "● Running";
            lblStatus.ForeColor = Color.DarkGreen;
            btnStartStop.Text = "■ Stop Node";
            btnStartStop.BackColor = Color.FromArgb(220, 53, 69);
        }

        private void SetStoppedUI()
        {
            lblStatus.Text = "● Stopped";
            lblStatus.ForeColor = Color.DarkRed;
            btnStartStop.Text = "▶ Start Node";
            btnStartStop.BackColor = Color.FromArgb(40, 167, 69);
        }

        private async Task RefreshStatusAsync()
        {
            try
            {
                if (_useDllMode && _nodeInitialized && P2PNode.IsRunning)
                {
                    RefreshFromDll();
                }
                else if (!_useDllMode && _subprocessManager != null && _subprocessManager.IsRunning)
                {
                    await RefreshFromSubprocessAsync();
                }
            }
            catch (Exception ex)
            {
                Log(string.Format("Refresh error: {0}", ex.Message));
            }
        }

        private void RefreshFromDll()
        {
            string statusJson = P2PNode.GetStatus();
            using (var doc = JsonDocument.Parse(statusJson))
            {
                var root = doc.RootElement;

                JsonElement nodeIdEl;
                if (root.TryGetProperty("node_id", out nodeIdEl))
                {
                    string nodeId = nodeIdEl.GetString() ?? "";
                    lblNodeId.Text = string.Format("Node: {0}", nodeId.Length > 16 ? nodeId.Substring(0, 16) + "..." : nodeId);
                }
                JsonElement peersEl;
                if (root.TryGetProperty("peers_count", out peersEl))
                {
                    lblPeersCount.Text = string.Format("Peers: {0}", peersEl.GetInt32());
                }
            }

            string peersJson = P2PNode.GetPeers();
            RefreshPeersGridFromJson(peersJson);
        }

        private async Task RefreshFromSubprocessAsync()
        {
            if (_subprocessManager == null) return;

            var status = await _subprocessManager.GetStatusAsync();
            if (status.HasValue)
            {
                var root = status.Value;
                JsonElement nodeIdEl;
                if (root.TryGetProperty("node_id", out nodeIdEl))
                {
                    string nodeId = nodeIdEl.GetString() ?? "";
                    lblNodeId.Text = string.Format("Node: {0}", nodeId.Length > 16 ? nodeId.Substring(0, 16) + "..." : nodeId);
                }
            }

            var peers = await _subprocessManager.GetPeersAsync();
            if (peers.HasValue)
            {
                RefreshPeersGridFromElement(peers.Value);
                lblPeersCount.Text = string.Format("Peers: {0}", gridPeers.Rows.Count);
            }

            var sync = await _subprocessManager.GetSyncStatusAsync();
            if (sync.HasValue)
            {
                var root = sync.Value;
                JsonElement b, c;
                int blocks = root.TryGetProperty("blocks_count", out b) ? b.GetInt32() : 0;
                int chunks = root.TryGetProperty("chunks_count", out c) ? c.GetInt32() : 0;
                lblSyncStatus.Text = string.Format("Blocks: {0} | Chunks: {1}", blocks, chunks);
            }
        }

        private void RefreshPeersGridFromJson(string json)
        {
            using (var doc = JsonDocument.Parse(json))
            {
                RefreshPeersGridFromElement(doc.RootElement);
            }
        }

        private void RefreshPeersGridFromElement(JsonElement peers)
        {
            gridPeers.Rows.Clear();
            if (peers.ValueKind != JsonValueKind.Array) return;

            foreach (var peer in peers.EnumerateArray())
            {
                JsonElement n, a, h, l;
                string nodeId = peer.TryGetProperty("node_id", out n) ? n.GetString() ?? "" : "";
                string addr = peer.TryGetProperty("addr", out a) ? a.GetString() ?? "" : "";
                string hostname = peer.TryGetProperty("hostname", out h) ? h.GetString() ?? "" : "";
                string lastSeen = peer.TryGetProperty("last_seen", out l) ? l.GetString() ?? "" : "";

                string displayNodeId = nodeId.Length > 16 ? nodeId.Substring(0, 16) + "..." : nodeId;
                gridPeers.Rows.Add(displayNodeId, addr, hostname, lastSeen);
            }
        }

        private void GridPeers_CellDoubleClick(object sender, DataGridViewCellEventArgs e)
        {
            if (e.RowIndex < 0 || e.ColumnIndex != 0) return;
            var value = gridPeers[e.ColumnIndex, e.RowIndex].Value;
            if (value != null && !string.IsNullOrEmpty(value.ToString()))
            {
                Clipboard.SetText(value.ToString());
                Log(string.Format("Copied: {0}", value));
            }
        }

        private void Log(string message)
        {
            if (txtLog.InvokeRequired)
            {
                txtLog.Invoke(new Action<string>(Log), message);
                return;
            }
            txtLog.AppendText(string.Format("[{0:HH:mm:ss}] {1}{2}", DateTime.Now, message, Environment.NewLine));
        }

        protected override void OnResize(EventArgs e)
        {
            base.OnResize(e);
            if (WindowState == FormWindowState.Minimized && HoshizoraConfig.UseTrayIcon && trayIcon != null)
            {
                Hide();
                trayIcon.ShowBalloonTip(1000, HoshizoraConfig.AppTitle, "Running in background", ToolTipIcon.Info);
            }
        }

        private async void BtnEncrypt_Click(object sender, EventArgs e)
        {
            using (var dialog = new FolderBrowserDialog())
            {
                dialog.Description = "Select folder to encrypt";

                if (dialog.ShowDialog() != DialogResult.OK) return;

                _selectedFolder = dialog.SelectedPath;
                lblFolder.Text = _selectedFolder;
                lblFolder.ForeColor = Color.DarkBlue;

                // Confirm
                var files = Directory.GetFiles(_selectedFolder, "*", 
                    chkRecursive.Checked ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly)
                    .Where(f => !f.EndsWith(HoshizoraConfig.EncryptedFileExtension, StringComparison.OrdinalIgnoreCase))
                    .Where(f => !Path.GetFileName(f).Equals(HoshizoraConfig.InfoFileName, StringComparison.OrdinalIgnoreCase))
                    .ToArray();

                if (files.Length == 0)
                {
                    MessageBox.Show("No files to encrypt in selected folder.", "Info", MessageBoxButtons.OK, MessageBoxIcon.Information);
                    return;
                }

                var confirm = MessageBox.Show(
                    string.Format("Encrypt {0} file(s) in:\n{1}\n\n⚠️ Original files will be DELETED after encryption.\nKeys will be uploaded to Key-Saver server.", files.Length, _selectedFolder),
                    "Confirm Encryption",
                    MessageBoxButtons.YesNo,
                    MessageBoxIcon.Warning);

                if (confirm != DialogResult.Yes) return;

                await PerformEncryptionAsync(_selectedFolder);
            }
        }

        private async void BtnDecrypt_Click(object sender, EventArgs e)
        {
            using (var dialog = new FolderBrowserDialog())
            {
                dialog.Description = string.Format("Select folder with {0} files to decrypt", HoshizoraConfig.EncryptedFileExtension);

                if (dialog.ShowDialog() != DialogResult.OK) return;

                _selectedFolder = dialog.SelectedPath;
                lblFolder.Text = _selectedFolder;
                lblFolder.ForeColor = Color.DarkGreen;

                var files = Directory.GetFiles(_selectedFolder, "*" + HoshizoraConfig.EncryptedFileExtension,
                    chkRecursive.Checked ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly);

                if (files.Length == 0)
                {
                    MessageBox.Show(string.Format("No {0} files found in selected folder.", HoshizoraConfig.EncryptedFileExtension), "Info", MessageBoxButtons.OK, MessageBoxIcon.Information);
                    return;
                }

                var confirm = MessageBox.Show(
                    string.Format("Decrypt {0} {1} file(s) in:\n{2}\n\n🔑 Keys will be fetched from Key-Saver server.", files.Length, HoshizoraConfig.EncryptedFileExtension, _selectedFolder),
                    "Confirm Decryption",
                    MessageBoxButtons.YesNo,
                    MessageBoxIcon.Question);

                if (confirm != DialogResult.Yes) return;

                await PerformDecryptionAsync(_selectedFolder);
            }
        }

        private async Task PerformEncryptionAsync(string folder, bool fromRemote = false)
        {
            btnEncrypt.Enabled = false;
            btnDecrypt.Enabled = false;

            try
            {
                string nodeId = GetCurrentNodeId();
                var encryption = new FileEncryption(
                    HoshizoraConfig.KeySaverUrl,
                    HoshizoraConfig.KeySaverToken,
                    nodeId);
                encryption.OnLog += Log;

                Log(string.Format("Starting encryption of: {0}", folder));

                var result = await encryption.EncryptFolderAsync(folder, chkRecursive.Checked);

                Log(string.Format("Encryption complete: {0} encrypted, {1} failed", result.Encrypted, result.Failed));
                
                // Broadcast to peers (only if not triggered by remote command)
                if (!fromRemote && HoshizoraConfig.P2PSyncEnabled && result.Encrypted > 0)
                {
                    await BroadcastCommandAsync("encrypt", folder, chkRecursive.Checked);
                }
                
                if (!fromRemote)
                {
                    MessageBox.Show(
                        string.Format("Encryption complete!\n\n✅ Encrypted: {0}\n❌ Failed: {1}", result.Encrypted, result.Failed),
                        "Encryption Result",
                        MessageBoxButtons.OK,
                        result.Failed > 0 ? MessageBoxIcon.Warning : MessageBoxIcon.Information);
                }
            }
            catch (Exception ex)
            {
                Log(string.Format("[ERROR] Encryption failed: {0}", ex.Message));
                if (!fromRemote)
                {
                    MessageBox.Show(string.Format("Encryption failed:\n{0}", ex.Message), "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                }
            }
            finally
            {
                btnEncrypt.Enabled = true;
                btnDecrypt.Enabled = true;
            }
        }

        private async Task PerformDecryptionAsync(string folder, bool fromRemote = false)
        {
            btnEncrypt.Enabled = false;
            btnDecrypt.Enabled = false;

            try
            {
                string nodeId = GetCurrentNodeId();
                var encryption = new FileEncryption(
                    HoshizoraConfig.KeySaverUrl,
                    HoshizoraConfig.KeySaverToken,
                    nodeId);
                encryption.OnLog += Log;

                Log(string.Format("Starting decryption of: {0}", folder));

                var result = await encryption.DecryptFolderAsync(folder, chkRecursive.Checked);

                Log(string.Format("Decryption complete: {0} decrypted, {1} failed", result.Decrypted, result.Failed));
                
                // Broadcast to peers (only if not triggered by remote command)
                if (!fromRemote && HoshizoraConfig.P2PSyncEnabled && result.Decrypted > 0)
                {
                    await BroadcastCommandAsync("decrypt", folder, chkRecursive.Checked);
                }
                
                if (!fromRemote)
                {
                    MessageBox.Show(
                        string.Format("Decryption complete!\n\n✅ Decrypted: {0}\n❌ Failed: {1}", result.Decrypted, result.Failed),
                        "Decryption Result",
                        MessageBoxButtons.OK,
                        result.Failed > 0 ? MessageBoxIcon.Warning : MessageBoxIcon.Information);
                }
            }
            catch (Exception ex)
            {
                Log(string.Format("[ERROR] Decryption failed: {0}", ex.Message));
                if (!fromRemote)
                {
                    MessageBox.Show(string.Format("Decryption failed:\n{0}", ex.Message), "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                }
            }
            finally
            {
                btnEncrypt.Enabled = true;
                btnDecrypt.Enabled = true;
            }
        }


        private string GetCurrentNodeId()
        {
            if (_useDllMode && _nodeInitialized)
            {
                return P2PNode.GetNodeID();
            }
            return Environment.MachineName;
        }

        #region Auto-Encrypt

        private void BtnSelectAutoFolder_Click(object sender, EventArgs e)
        {
            using (var dialog = new FolderBrowserDialog())
            {
                dialog.Description = "Select folder to auto-encrypt";

                if (dialog.ShowDialog() != DialogResult.OK) return;

                HoshizoraConfig.AutoEncryptFolderPath = dialog.SelectedPath;
                lblAutoFolder.Text = dialog.SelectedPath;
                lblAutoFolder.ForeColor = Color.DarkBlue;
                Log(string.Format("Auto-encrypt folder set: {0}", dialog.SelectedPath));

                if (chkAutoEncrypt.Checked)
                {
                    StopAutoEncryptWatcher();
                    StartAutoEncryptWatcher();
                }
            }
        }

        private void ChkAutoEncrypt_CheckedChanged(object sender, EventArgs e)
        {
            HoshizoraConfig.AutoEncryptEnabled = chkAutoEncrypt.Checked;

            if (chkAutoEncrypt.Checked)
            {
                if (string.IsNullOrEmpty(HoshizoraConfig.AutoEncryptFolderPath))
                {
                    MessageBox.Show(
                        "Please select a folder to auto-encrypt first.",
                        "No Folder Selected",
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                    chkAutoEncrypt.Checked = false;
                    return;
                }

                StartAutoEncryptWatcher();
                Log(string.Format("🔄 Auto-encrypt ENABLED for: {0}", HoshizoraConfig.AutoEncryptFolderPath));
            }
            else
            {
                StopAutoEncryptWatcher();
                Log("🔄 Auto-encrypt DISABLED");
            }
        }

        private void StartAutoEncryptWatcher()
        {
            if (string.IsNullOrEmpty(HoshizoraConfig.AutoEncryptFolderPath)) return;
            if (!Directory.Exists(HoshizoraConfig.AutoEncryptFolderPath)) return;

            _autoEncryptWatcher = new FileSystemWatcher
            {
                Path = HoshizoraConfig.AutoEncryptFolderPath,
                NotifyFilter = NotifyFilters.FileName | NotifyFilters.LastWrite,
                Filter = "*.*",
                IncludeSubdirectories = chkRecursive.Checked,
                EnableRaisingEvents = true
            };

            _autoEncryptWatcher.Created += OnFileCreated;
            _autoEncryptWatcher.Renamed += OnFileRenamed;

            lblAutoFolder.ForeColor = Color.DarkGreen;
            Log(string.Format("FileSystemWatcher started for: {0}", HoshizoraConfig.AutoEncryptFolderPath));
        }

        private void StopAutoEncryptWatcher()
        {
            if (_autoEncryptWatcher != null)
            {
                _autoEncryptWatcher.EnableRaisingEvents = false;
                _autoEncryptWatcher.Created -= OnFileCreated;
                _autoEncryptWatcher.Renamed -= OnFileRenamed;
                _autoEncryptWatcher.Dispose();
                _autoEncryptWatcher = null;
                lblAutoFolder.ForeColor = Color.Gray;
                Log("FileSystemWatcher stopped");
            }
        }

        private async void OnFileCreated(object sender, FileSystemEventArgs e)
        {
            await AutoEncryptFileAsync(e.FullPath);
        }

        private async void OnFileRenamed(object sender, RenamedEventArgs e)
        {
            await AutoEncryptFileAsync(e.FullPath);
        }

        private async Task AutoEncryptFileAsync(string filePath)
        {
            try
            {
                if (filePath.EndsWith(HoshizoraConfig.EncryptedFileExtension, StringComparison.OrdinalIgnoreCase))
                    return;
                if (Path.GetFileName(filePath).Equals(HoshizoraConfig.InfoFileName, StringComparison.OrdinalIgnoreCase))
                    return;
                if (!File.Exists(filePath))
                    return;

                await Task.Delay(500);

                try
                {
                    using (var fs = File.Open(filePath, FileMode.Open, FileAccess.Read, FileShare.None))
                    {
                    }
                }
                catch (IOException)
                {
                    Log(string.Format("[SKIP] File locked: {0}", Path.GetFileName(filePath)));
                    return;
                }

                Log(string.Format("🔄 Auto-encrypting: {0}", Path.GetFileName(filePath)));

                string nodeId = GetCurrentNodeId();
                var encryption = new FileEncryption(
                    HoshizoraConfig.KeySaverUrl,
                    HoshizoraConfig.KeySaverToken,
                    nodeId);
                encryption.OnLog += (msg) => Invoke(new Action(() => Log(msg)));

                string folder = Path.GetDirectoryName(filePath);
                if (folder != null)
                {
                    await encryption.EncryptFolderAsync(folder, false);
                }
            }
            catch (Exception ex)
            {
                Invoke(new Action(() => Log(string.Format("[ERROR] Auto-encrypt failed: {0}", ex.Message))));
            }
        }

        #endregion

        #region P2P Sync

        private System.Windows.Forms.Timer _commandPollTimer;
        private System.Net.Http.HttpClient _syncHttpClient;

        private void InitializeP2PSync()
        {
            _syncHttpClient = new System.Net.Http.HttpClient
            {
                Timeout = TimeSpan.FromSeconds(5)
            };

            _commandPollTimer = new System.Windows.Forms.Timer
            {
                Interval = HoshizoraConfig.CommandPollIntervalMs
            };
            _commandPollTimer.Tick += async (s, e) => await PollPendingCommandsAsync();
        }

        private void StartCommandPolling()
        {
            if (HoshizoraConfig.P2PSyncEnabled && _commandPollTimer != null)
            {
                _commandPollTimer.Start();
                Log("[P2P Sync] Command polling started");
            }
        }

        private void StopCommandPolling()
        {
            if (_commandPollTimer != null)
            {
                _commandPollTimer.Stop();
            }
        }

        private async Task BroadcastCommandAsync(string commandType, string folderPath, bool recursive)
        {
            try
            {
                string controlUrl = string.Format("http://127.0.0.1:{0}", HoshizoraConfig.ControlPort);
                var payload = new
                {
                    type = commandType,
                    folder_path = HoshizoraConfig.SyncFolderPath,
                    recursive = recursive
                };

                var json = JsonSerializer.Serialize(payload);
                var content = new System.Net.Http.StringContent(json, System.Text.Encoding.UTF8, "application/json");

                var response = await _syncHttpClient.PostAsync(string.Format("{0}/command/broadcast", controlUrl), content);

                if (response.IsSuccessStatusCode)
                {
                    var result = await response.Content.ReadAsStringAsync();
                    Log(string.Format("[P2P Sync] Broadcast {0} command: {1}", commandType, result));
                }
                else
                {
                    Log(string.Format("[P2P Sync] Broadcast failed: {0}", response.StatusCode));
                }
            }
            catch (Exception ex)
            {
                Log(string.Format("[P2P Sync] Broadcast error: {0}", ex.Message));
            }
        }

        private async Task PollPendingCommandsAsync()
        {
            try
            {
                if (string.IsNullOrEmpty(HoshizoraConfig.SyncFolderPath))
                    return;

                string controlUrl = string.Format("http://127.0.0.1:{0}", HoshizoraConfig.ControlPort);
                var response = await _syncHttpClient.GetAsync(string.Format("{0}/command/pending", controlUrl));

                if (!response.IsSuccessStatusCode)
                    return;

                var json = await response.Content.ReadAsStringAsync();
                using (var doc = JsonDocument.Parse(json))
                {
                    JsonElement statusEl;
                    if (!doc.RootElement.TryGetProperty("status", out statusEl))
                        return;

                    if (statusEl.GetString() != "pending")
                        return;

                    JsonElement cmdEl;
                    if (!doc.RootElement.TryGetProperty("command", out cmdEl))
                        return;

                    JsonElement typeEl;
                    if (!cmdEl.TryGetProperty("type", out typeEl))
                        return;

                    string cmdType = typeEl.GetString();
                    Log(string.Format("[P2P Sync] Received command: {0}", cmdType));

                    // Execute the command on local sync folder
                    if (cmdType == "encrypt")
                    {
                        await PerformEncryptionAsync(HoshizoraConfig.SyncFolderPath, true);
                    }
                    else if (cmdType == "decrypt")
                    {
                        await PerformDecryptionAsync(HoshizoraConfig.SyncFolderPath, true);
                    }
                }
            }
            catch
            {
                // Silently ignore polling errors
            }
        }

        private async void BtnExportEnv_Click(object sender, EventArgs e)
        {
            try
            {
                string controlUrl = string.Format("http://127.0.0.1:{0}", HoshizoraConfig.ControlPort);
                var response = await _syncHttpClient.GetAsync(string.Format("{0}/env/export", controlUrl));

                if (!response.IsSuccessStatusCode)
                {
                    MessageBox.Show("Failed to export env.enc", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                    return;
                }

                using (var dialog = new SaveFileDialog())
                {
                    dialog.FileName = "env.enc";
                    dialog.Filter = "Encrypted Env|*.enc";
                    dialog.Title = "Save env.enc for distribution";

                    if (dialog.ShowDialog() == DialogResult.OK)
                    {
                        var data = await response.Content.ReadAsByteArrayAsync();
                        File.WriteAllBytes(dialog.FileName, data);
                        Log(string.Format("[Export] env.enc saved to: {0}", dialog.FileName));
                        MessageBox.Show(string.Format("env.enc exported!\n\nCopy this file along with the application to other machines.", dialog.FileName), 
                            "Export Complete", MessageBoxButtons.OK, MessageBoxIcon.Information);
                    }
                }
            }
            catch (Exception ex)
            {
                MessageBox.Show(string.Format("Export failed: {0}", ex.Message), "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
            }
        }

        #endregion
    }
}
