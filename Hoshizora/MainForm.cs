using System.Text.Json;

namespace Hoshizora;

public class MainForm : Form
{
    private Label lblStatus = null!;
    private Label lblNodeId = null!;
    private Label lblPeersCount = null!;
    private Label lblSyncStatus = null!;
    private Label lblMode = null!;
    private Button btnStartStop = null!;
    private Button btnRefresh = null!;
    private DataGridView gridPeers = null!;
    private TextBox txtLog = null!;
    private System.Windows.Forms.Timer refreshTimer = null!;
    private NotifyIcon? trayIcon;

    // Two modes: DLL (preferred) or Subprocess (fallback)
    private bool _useDllMode = true;
    private bool _dllLoaded = false;
    private bool _nodeInitialized = false;
    private P2PNodeManager? _subprocessManager;

    // Encryption UI
    private Button btnEncrypt = null!;
    private Button btnDecrypt = null!;
    private CheckBox chkRecursive = null!;
    private Label lblFolder = null!;
    private string? _selectedFolder;

    public MainForm()
    {
        Text = HoshizoraConfig.AppTitle;
        Width = 900;
        Height = 680;
        StartPosition = FormStartPosition.CenterScreen;
        Icon = SystemIcons.Application;

        InitializeLayout();
        InitializeTimer();

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
            RowCount = 5,
            ColumnCount = 1,
            Padding = new Padding(10)
        };
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 45));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 25));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 30));
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
        root.Controls.Add(pnlPeers, 0, 2);

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
            Text = $"""
                API Port: {HoshizoraConfig.ApiPort}  |  Control Port: {HoshizoraConfig.ControlPort}
                Multicast: {HoshizoraConfig.MulticastGroup}:{HoshizoraConfig.MulticastPort}
                Key-Saver: {HoshizoraConfig.KeySaverUrl}
                """
        };
        pnlConfig.Controls.Add(configInfo);
        root.Controls.Add(pnlConfig, 0, 3);

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
        root.Controls.Add(pnlLog, 0, 4);
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
        menu.Items.Add("Start/Stop", null, (s, e) => BtnStartStop_Click(s, e!));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Exit", null, (s, e) => Application.Exit());

        trayIcon.ContextMenuStrip = menu;
        trayIcon.DoubleClick += (s, e) => { Show(); WindowState = FormWindowState.Normal; };
    }

    private async void MainForm_Load(object? sender, EventArgs e)
    {
        Log("Hoshizora starting...");

        // Try DLL first
        string dllPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "p2pnode.dll");
        if (File.Exists(dllPath))
        {
            try
            {
                // Test if DLL can be loaded
                _ = P2PNode.P2P_IsRunning();
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
                Log($"[WARNING] DLL load error: {ex.Message} - using subprocess mode");
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

    private void MainForm_FormClosing(object? sender, FormClosingEventArgs e)
    {
        refreshTimer.Stop();

        if (_useDllMode && _nodeInitialized)
        {
            try { P2PNode.P2P_Stop(); } catch { }
        }

        _subprocessManager?.Dispose();
        trayIcon?.Dispose();
    }

    private async void BtnStartStop_Click(object? sender, EventArgs e)
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
            Log($"[ERROR] {ex.Message}");
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
                // forceNewEnv=1 to recreate env.enc with Hoshizora's passphrase
                int result = P2PNode.P2P_Init(
                    HoshizoraConfig.EnvPassphrase,
                    HoshizoraConfig.ApiPort,
                    HoshizoraConfig.ControlPort,
                    HoshizoraConfig.MulticastGroup,
                    HoshizoraConfig.MulticastPort,
                    HoshizoraConfig.KeySaverUrl,
                    1); // forceNewEnv=1: always use Hoshizora's passphrase

                if (result != 0)
                {
                    Log($"[ERROR] Init failed (code: {result})");
                    MessageBox.Show($"Node init failed (code: {result})", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                    return;
                }
                _nodeInitialized = true;
                Log("Node initialized");
            }

            Log("Starting node (DLL)...");
            int startResult = P2PNode.P2P_Start();
            if (startResult != 0)
            {
                Log($"[ERROR] Start failed (code: {startResult})");
                MessageBox.Show($"Node start failed (code: {startResult})", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
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
            else if (!_useDllMode && _subprocessManager?.IsRunning == true)
            {
                await RefreshFromSubprocessAsync();
            }
        }
        catch (Exception ex)
        {
            Log($"Refresh error: {ex.Message}");
        }
    }

    private void RefreshFromDll()
    {
        string statusJson = P2PNode.GetStatus();
        using var doc = JsonDocument.Parse(statusJson);
        var root = doc.RootElement;

        if (root.TryGetProperty("node_id", out var nodeIdEl))
        {
            string nodeId = nodeIdEl.GetString() ?? "";
            lblNodeId.Text = $"Node: {(nodeId.Length > 16 ? nodeId[..16] + "..." : nodeId)}";
        }
        if (root.TryGetProperty("peers_count", out var peersEl))
        {
            lblPeersCount.Text = $"Peers: {peersEl.GetInt32()}";
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
            if (root.TryGetProperty("node_id", out var nodeIdEl))
            {
                string nodeId = nodeIdEl.GetString() ?? "";
                lblNodeId.Text = $"Node: {(nodeId.Length > 16 ? nodeId[..16] + "..." : nodeId)}";
            }
        }

        var peers = await _subprocessManager.GetPeersAsync();
        if (peers.HasValue)
        {
            RefreshPeersGridFromElement(peers.Value);
            lblPeersCount.Text = $"Peers: {gridPeers.Rows.Count}";
        }

        var sync = await _subprocessManager.GetSyncStatusAsync();
        if (sync.HasValue)
        {
            var root = sync.Value;
            int blocks = root.TryGetProperty("blocks_count", out var b) ? b.GetInt32() : 0;
            int chunks = root.TryGetProperty("chunks_count", out var c) ? c.GetInt32() : 0;
            lblSyncStatus.Text = $"Blocks: {blocks} | Chunks: {chunks}";
        }
    }

    private void RefreshPeersGridFromJson(string json)
    {
        using var doc = JsonDocument.Parse(json);
        RefreshPeersGridFromElement(doc.RootElement);
    }

    private void RefreshPeersGridFromElement(JsonElement peers)
    {
        gridPeers.Rows.Clear();
        if (peers.ValueKind != JsonValueKind.Array) return;

        foreach (var peer in peers.EnumerateArray())
        {
            string nodeId = peer.TryGetProperty("node_id", out var n) ? n.GetString() ?? "" : "";
            string addr = peer.TryGetProperty("addr", out var a) ? a.GetString() ?? "" : "";
            string hostname = peer.TryGetProperty("hostname", out var h) ? h.GetString() ?? "" : "";
            string lastSeen = peer.TryGetProperty("last_seen", out var l) ? l.GetString() ?? "" : "";

            string displayNodeId = nodeId.Length > 16 ? nodeId[..16] + "..." : nodeId;
            gridPeers.Rows.Add(displayNodeId, addr, hostname, lastSeen);
        }
    }

    private void GridPeers_CellDoubleClick(object? sender, DataGridViewCellEventArgs e)
    {
        if (e.RowIndex < 0 || e.ColumnIndex != 0) return;
        var value = gridPeers[e.ColumnIndex, e.RowIndex].Value?.ToString();
        if (!string.IsNullOrEmpty(value))
        {
            Clipboard.SetText(value);
            Log($"Copied: {value}");
        }
    }

    private void Log(string message)
    {
        if (txtLog.InvokeRequired)
        {
            txtLog.Invoke(new Action<string>(Log), message);
            return;
        }
        txtLog.AppendText($"[{DateTime.Now:HH:mm:ss}] {message}{Environment.NewLine}");
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

    private async void BtnEncrypt_Click(object? sender, EventArgs e)
    {
        using var dialog = new FolderBrowserDialog
        {
            Description = "Select folder to encrypt",
            UseDescriptionForTitle = true
        };

        if (dialog.ShowDialog() != DialogResult.OK) return;

        _selectedFolder = dialog.SelectedPath;
        lblFolder.Text = _selectedFolder;
        lblFolder.ForeColor = Color.DarkBlue;

        // Confirm
        var files = Directory.GetFiles(_selectedFolder, "*", 
            chkRecursive.Checked ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly)
            .Where(f => !f.EndsWith(".henc")).ToArray();

        if (files.Length == 0)
        {
            MessageBox.Show("No files to encrypt in selected folder.", "Info", MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        var confirm = MessageBox.Show(
            $"Encrypt {files.Length} file(s) in:\n{_selectedFolder}\n\n⚠️ Original files will be DELETED after encryption.\nKeys will be uploaded to Key-Saver server.",
            "Confirm Encryption",
            MessageBoxButtons.YesNo,
            MessageBoxIcon.Warning);

        if (confirm != DialogResult.Yes) return;

        await PerformEncryptionAsync(_selectedFolder);
    }

    private async void BtnDecrypt_Click(object? sender, EventArgs e)
    {
        using var dialog = new FolderBrowserDialog
        {
            Description = "Select folder with .henc files to decrypt",
            UseDescriptionForTitle = true
        };

        if (dialog.ShowDialog() != DialogResult.OK) return;

        _selectedFolder = dialog.SelectedPath;
        lblFolder.Text = _selectedFolder;
        lblFolder.ForeColor = Color.DarkGreen;

        var files = Directory.GetFiles(_selectedFolder, "*.henc",
            chkRecursive.Checked ? SearchOption.AllDirectories : SearchOption.TopDirectoryOnly);

        if (files.Length == 0)
        {
            MessageBox.Show("No .henc files found in selected folder.", "Info", MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        var confirm = MessageBox.Show(
            $"Decrypt {files.Length} .henc file(s) in:\n{_selectedFolder}\n\n🔑 Keys will be fetched from Key-Saver server.",
            "Confirm Decryption",
            MessageBoxButtons.YesNo,
            MessageBoxIcon.Question);

        if (confirm != DialogResult.Yes) return;

        await PerformDecryptionAsync(_selectedFolder);
    }

    private async Task PerformEncryptionAsync(string folder)
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

            Log($"Starting encryption of: {folder}");

            var (encrypted, failed) = await encryption.EncryptFolderAsync(folder, chkRecursive.Checked);

            Log($"Encryption complete: {encrypted} encrypted, {failed} failed");
            MessageBox.Show(
                $"Encryption complete!\n\n✅ Encrypted: {encrypted}\n❌ Failed: {failed}",
                "Encryption Result",
                MessageBoxButtons.OK,
                failed > 0 ? MessageBoxIcon.Warning : MessageBoxIcon.Information);
        }
        catch (Exception ex)
        {
            Log($"[ERROR] Encryption failed: {ex.Message}");
            MessageBox.Show($"Encryption failed:\n{ex.Message}", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
        finally
        {
            btnEncrypt.Enabled = true;
            btnDecrypt.Enabled = true;
        }
    }

    private async Task PerformDecryptionAsync(string folder)
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

            Log($"Starting decryption of: {folder}");

            var (decrypted, failed) = await encryption.DecryptFolderAsync(folder, chkRecursive.Checked);

            Log($"Decryption complete: {decrypted} decrypted, {failed} failed");
            MessageBox.Show(
                $"Decryption complete!\n\n✅ Decrypted: {decrypted}\n❌ Failed: {failed}",
                "Decryption Result",
                MessageBoxButtons.OK,
                failed > 0 ? MessageBoxIcon.Warning : MessageBoxIcon.Information);
        }
        catch (Exception ex)
        {
            Log($"[ERROR] Decryption failed: {ex.Message}");
            MessageBox.Show($"Decryption failed:\n{ex.Message}", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
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
        // Fallback: use machine name as node ID
        return Environment.MachineName;
    }
}
