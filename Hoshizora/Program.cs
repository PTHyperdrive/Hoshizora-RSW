namespace Hoshizora;

static class Program
{
    /// <summary>
    /// The main entry point for the application.
    /// </summary>
    [STAThread]
    static void Main()
    {
        // Enable visual styles
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        Application.SetHighDpiMode(HighDpiMode.SystemAware);

        // Handle unhandled exceptions
        Application.ThreadException += (sender, e) =>
        {
            MessageBox.Show(
                $"An error occurred:\n\n{e.Exception.Message}",
                "Hoshizora Error",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
        };

        AppDomain.CurrentDomain.UnhandledException += (sender, e) =>
        {
            if (e.ExceptionObject is Exception ex)
            {
                MessageBox.Show(
                    $"A fatal error occurred:\n\n{ex.Message}",
                    "Hoshizora Fatal Error",
                    MessageBoxButtons.OK,
                    MessageBoxIcon.Error);
            }
        };

        // Run the application
        Application.Run(new MainForm());
    }
}
