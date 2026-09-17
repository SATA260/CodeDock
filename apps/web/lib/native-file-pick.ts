import { execFile } from "node:child_process";
import { stat } from "node:fs/promises";
import { homedir, platform } from "node:os";
import { dirname } from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export async function pickFilesNative(options: {
  images?: boolean;
  multiple?: boolean;
  start?: string;
}): Promise<string[]> {
  const start = await existingDir(options.start);
  switch (platform()) {
    case "darwin":
      return pickMacFiles({ ...options, start });
    case "linux":
      return pickLinuxFiles({ ...options, start });
    case "win32":
      return pickWindowsFiles({ ...options, start });
    default:
      throw new Error("当前系统不支持弹出文件选择框");
  }
}

export async function pickDirectoryNative(options: { start?: string } = {}): Promise<string | undefined> {
  const start = await existingDir(options.start);
  switch (platform()) {
    case "darwin":
      return pickMacDirectory(start);
    case "linux":
      return pickLinuxDirectory(start);
    case "win32":
      return pickWindowsDirectory(start);
    default:
      throw new Error("当前系统不支持弹出目录选择框");
  }
}

async function existingDir(path?: string): Promise<string | undefined> {
  const raw = path?.trim();
  if (!raw) {
    return homedir();
  }
  try {
    const info = await stat(raw);
    if (info.isDirectory()) {
      return raw;
    }
    if (info.isFile()) {
      return dirname(raw);
    }
  } catch {
    // fall through
  }
  return homedir();
}

function splitPaths(stdout: string): string[] {
  return stdout
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

async function pickMacFiles(options: { images?: boolean; multiple?: boolean; start?: string }): Promise<string[]> {
  const prompt = options.images ? "选择图片" : "选择要挂上的文件";
  const typeClause = options.images ? ' of type {"public.image"}' : "";
  const multiClause = options.multiple ? " with multiple selections allowed" : "";
  const defaultClause = options.start
    ? ` default location POSIX file ${appleString(options.start)}`
    : "";
  const script = `
try
  set theChoice to choose file with prompt ${appleString(prompt)}${typeClause}${defaultClause}${multiClause}
  set output to ""
  if class of theChoice is list then
    repeat with f in theChoice
      set output to output & POSIX path of f & linefeed
    end repeat
  else
    set output to POSIX path of theChoice
  end if
  return output
on error number -128
  return ""
end try
`;
  try {
    const { stdout } = await execFileAsync("osascript", ["-e", script], { timeout: 300_000 });
    return splitPaths(stdout);
  } catch (err) {
    throw nativePickError(err, "无法打开系统文件选择框");
  }
}

async function pickMacDirectory(start?: string): Promise<string | undefined> {
  const defaultClause = start ? ` default location POSIX file ${appleString(start)}` : "";
  const script = `
try
  set theChoice to choose folder with prompt ${appleString("选择工作目录")}${defaultClause}
  return POSIX path of theChoice
on error number -128
  return ""
end try
`;
  try {
    const { stdout } = await execFileAsync("osascript", ["-e", script], { timeout: 300_000 });
    return firstPath(stdout);
  } catch {
    throw new Error("无法打开系统目录选择框");
  }
}

async function pickLinuxFiles(options: { images?: boolean; multiple?: boolean; start?: string }): Promise<string[]> {
  const args = ["--file-selection", "--separator=\n"];
  if (options.multiple) {
    args.push("--multiple");
  }
  if (options.images) {
    args.push("--file-filter=图片 | *.png *.jpg *.jpeg *.gif *.webp *.heic *.bmp *.svg");
  }
  if (options.start) {
    args.push(`--filename=${options.start.replace(/\/?$/, "/")}`);
  }
  try {
    const { stdout } = await execFileAsync("zenity", args, { timeout: 300_000 });
    return splitPaths(stdout);
  } catch (err) {
    if (isCancel(err)) {
      return [];
    }
    throw nativePickError(err, "无法打开系统文件选择框（需要 zenity）");
  }
}

async function pickLinuxDirectory(start?: string): Promise<string | undefined> {
  const args = ["--file-selection", "--directory"];
  if (start) {
    args.push(`--filename=${start.replace(/\/?$/, "/")}`);
  }
  try {
    const { stdout } = await execFileAsync("zenity", args, { timeout: 300_000 });
    return firstPath(stdout);
  } catch (err) {
    if (isCancel(err)) {
      return undefined;
    }
    throw new Error("无法打开系统目录选择框（需要 zenity）");
  }
}

async function pickWindowsFiles(options: { images?: boolean; multiple?: boolean; start?: string }): Promise<string[]> {
  const filter = options.images
    ? "Images (*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp)|*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp|All files (*.*)|*.*"
    : "All files (*.*)|*.*";
  const script = `
Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Multiselect = $${options.multiple ? "true" : "false"}
$dialog.Filter = ${psString(filter)}
${options.start ? `$dialog.InitialDirectory = ${psString(options.start)}` : ""}
if ($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) { exit 0 }
$dialog.FileNames -join [Environment]::NewLine
`;
  try {
    const { stdout } = await execFileAsync(
      "powershell",
      ["-NoProfile", "-NonInteractive", "-Command", script],
      { timeout: 300_000 },
    );
    return splitPaths(stdout);
  } catch (err) {
    throw nativePickError(err, "无法打开系统文件选择框");
  }
}

async function pickWindowsDirectory(start?: string): Promise<string | undefined> {
  const script = `
Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = '选择工作目录'
${start ? `$dialog.SelectedPath = ${psString(start)}` : ""}
if ($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) { exit 0 }
$dialog.SelectedPath
`;
  try {
    const { stdout } = await execFileAsync(
      "powershell",
      ["-NoProfile", "-NonInteractive", "-Command", script],
      { timeout: 300_000 },
    );
    return firstPath(stdout);
  } catch {
    throw new Error("无法打开系统目录选择框");
  }
}

function firstPath(stdout: string): string | undefined {
  const path = splitPaths(stdout)[0];
  if (!path) {
    return undefined;
  }
  if (path === "/" || /^[A-Za-z]:[\\/]?$/.test(path)) {
    return path;
  }
  return path.replace(/[/\\]+$/, "");
}

function appleString(value: string): string {
  return `"${value.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}

function psString(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function isCancel(err: unknown): boolean {
  return Boolean(err && typeof err === "object" && "code" in err && (err as { code?: number }).code === 1);
}

function nativePickError(_err: unknown, fallback: string): Error {
  return new Error(fallback);
}
