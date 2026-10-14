param(
    [string]$ExePath = "D:\git\KiroX\build\bin\kirox.exe",
    [string]$WorkDir = "D:\git\KiroX"
)

Add-Type @"
using System;
using System.Runtime.InteropServices;

public class DesktopLauncher {
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    public struct STARTUPINFO {
        public Int32 cb;
        public string lpReserved;
        public string lpDesktop;
        public string lpTitle;
        public Int32 dwX;
        public Int32 dwY;
        public Int32 dwXSize;
        public Int32 dwYSize;
        public Int32 dwXCountChars;
        public Int32 dwYCountChars;
        public Int32 dwFillAttribute;
        public Int32 dwFlags;
        public Int16 wShowWindow;
        public Int16 cbReserved2;
        public IntPtr lpReserved2;
        public IntPtr hStdInput;
        public IntPtr hStdOutput;
        public IntPtr hStdError;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct PROCESS_INFORMATION {
        public IntPtr hProcess;
        public IntPtr hThread;
        public int dwProcessId;
        public int dwThreadId;
    }

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern bool CreateProcess(
        string lpApplicationName,
        string lpCommandLine,
        IntPtr lpProcessAttributes,
        IntPtr lpThreadAttributes,
        bool bInheritHandles,
        uint dwCreationFlags,
        IntPtr lpEnvironment,
        string lpCurrentDirectory,
        ref STARTUPINFO lpStartupInfo,
        out PROCESS_INFORMATION lpProcessInformation
    );

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool CloseHandle(IntPtr hObject);

    public static int Launch(string exePath, string workDir) {
        STARTUPINFO si = new STARTUPINFO();
        si.cb = Marshal.SizeOf(si);
        si.lpDesktop = @"WinSta0\Default";
        PROCESS_INFORMATION pi = new PROCESS_INFORMATION();

        string cmdLine = "\"" + exePath + "\"";
        bool success = CreateProcess(
            null,
            cmdLine,
            IntPtr.Zero,
            IntPtr.Zero,
            false,
            0,
            IntPtr.Zero,
            workDir,
            ref si,
            out pi
        );

        if (!success) {
            int err = Marshal.GetLastWin32Error();
            throw new Exception("CreateProcess failed with error code: " + err);
        }

        CloseHandle(pi.hProcess);
        CloseHandle(pi.hThread);
        return pi.dwProcessId;
    }
}
"@

$procId = [DesktopLauncher]::Launch($ExePath, $WorkDir)
Write-Output "Successfully launched $ExePath on WinSta0\Default with PID: $procId"
return $procId

