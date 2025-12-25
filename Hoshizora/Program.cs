using System;
using System.Threading;
using System.Windows.Forms;

namespace Hoshizora
{
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

            // Handle unhandled exceptions
            Application.ThreadException += (sender, e) =>
            {
                MessageBox.Show(
                    string.Format("An error occurred:\n\n{0}", e.Exception.Message),
                    "Hoshizora Error",
                    MessageBoxButtons.OK,
                    MessageBoxIcon.Error);
            };

            AppDomain.CurrentDomain.UnhandledException += (sender, e) =>
            {
                var ex = e.ExceptionObject as Exception;
                if (ex != null)
                {
                    MessageBox.Show(
                        string.Format("A fatal error occurred:\n\n{0}", ex.Message),
                        "Hoshizora Fatal Error",
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Error);
                }
            };

            // Run the application
            Application.Run(new MainForm());
        }
    }
}
