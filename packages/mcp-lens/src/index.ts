#!/usr/bin/env node

import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import * as crypto from "crypto";
import { spawn } from "child_process";
import * as https from "https";

// Platform detection and mapping
interface PlatformInfo {
  os: "darwin" | "linux" | "windows";
  arch: "arm64" | "amd64" | "x86_64";
}

function detectPlatform(): PlatformInfo {
  const platform = process.platform;
  const arch = process.arch;

  let osType: "darwin" | "linux" | "windows";
  if (platform === "darwin") {
    osType = "darwin";
  } else if (platform === "linux") {
    osType = "linux";
  } else if (platform === "win32") {
    osType = "windows";
  } else {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  let archType: "arm64" | "amd64" | "x86_64";
  if (arch === "arm64") {
    archType = "arm64";
  } else if (arch === "x64") {
    archType = "amd64";
  } else if ((arch as string) === "x86" || arch === "ia32") {
    archType = "x86_64";
  } else {
    throw new Error(`Unsupported architecture: ${arch}`);
  }

  return { os: osType, arch: archType };
}

// Derive release tag from package version
function getReleaseTag(): string {
  // Read package.json to get version
  const packageJsonPath = path.join(__dirname, "../package.json");
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf-8"));
  const version = packageJson.version;

  // Format as vX.Y.Z
  return `v${version}`;
}

// Get cache directory
function getCacheDir(): string {
  const cacheDir = path.join(os.homedir(), ".cache", "mcp-lens");
  if (!fs.existsSync(cacheDir)) {
    fs.mkdirSync(cacheDir, { recursive: true });
  }
  return cacheDir;
}

// Download file from URL
function downloadFile(url: string): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];

    https
      .get(url, (response) => {
        if (response.statusCode === 301 || response.statusCode === 302) {
          // Handle redirects
          if (response.headers.location) {
            downloadFile(response.headers.location).then(resolve).catch(reject);
            return;
          }
        }

        if (response.statusCode !== 200) {
          reject(
            new Error(`Failed to download: ${url} (${response.statusCode})`)
          );
          return;
        }

        response.on("data", (chunk: Buffer) => chunks.push(chunk));
        response.on("end", () => resolve(Buffer.concat(chunks)));
        response.on("error", reject);
      })
      .on("error", reject);
  });
}

// Verify SHA256 checksum
function verifySha256(data: Buffer, expectedHash: string): boolean {
  const hash = crypto.createHash("sha256").update(data).digest("hex");
  return hash.toLowerCase() === expectedHash.toLowerCase();
}

// Parse checksums.txt file
function parseChecksums(checksumContent: string): Map<string, string> {
  const checksums = new Map<string, string>();
  const lines = checksumContent.trim().split("\n");

  for (const line of lines) {
    const parts = line.trim().split(/\s+/);
    if (parts.length >= 2) {
      const hash = parts[0];
      const filename = parts[1];
      checksums.set(filename, hash);
    }
  }

  return checksums;
}

// Determine binary filename based on platform
function getBinaryFilename(platform: PlatformInfo): string {
  const osStr =
    platform.os === "windows"
      ? "windows"
      : platform.os === "darwin"
        ? "darwin"
        : "linux";

  const archStr =
    platform.arch === "arm64" ? "arm64" : platform.arch === "amd64" ? "amd64" : "x86_64";

  const ext = platform.os === "windows" ? ".exe" : "";

  return `mcp-lens_${osStr}_${archStr}${ext}`;
}

// Main function
async function main(): Promise<void> {
  try {
    // Detect platform
    const platform = detectPlatform();
    console.error(`[mcp-lens] Detected platform: ${platform.os} ${platform.arch}`);

    // Get release tag from version
    const releaseTag = getReleaseTag();
    console.error(`[mcp-lens] Using release tag: ${releaseTag}`);

    // Get cache directory
    const cacheDir = getCacheDir();
    const binaryFilename = getBinaryFilename(platform);
    const cachedBinaryPath = path.join(cacheDir, binaryFilename);

    // Check if binary is already cached
    if (fs.existsSync(cachedBinaryPath)) {
      console.error(`[mcp-lens] Using cached binary: ${cachedBinaryPath}`);
      execBinary(cachedBinaryPath);
      return;
    }

    // Download checksums.txt and binary
    const checksumUrl = `https://github.com/golovatskygroup/mcp-lens/releases/download/${releaseTag}/checksums.txt`;
    const binaryUrl = `https://github.com/golovatskygroup/mcp-lens/releases/download/${releaseTag}/${binaryFilename}`;

    console.error(`[mcp-lens] Downloading from ${releaseTag}...`);

    const checksumContent = await downloadFile(checksumUrl);
    const checksumText = checksumContent.toString("utf-8");
    const checksums = parseChecksums(checksumText);

    if (!checksums.has(binaryFilename)) {
      throw new Error(`Binary ${binaryFilename} not found in checksums.txt`);
    }

    const expectedHash = checksums.get(binaryFilename)!;

    console.error(`[mcp-lens] Downloading binary...`);
    const binaryData = await downloadFile(binaryUrl);

    // Verify checksum
    console.error(`[mcp-lens] Verifying checksum...`);
    if (!verifySha256(binaryData, expectedHash)) {
      throw new Error(
        `Checksum verification failed for ${binaryFilename}. Expected ${expectedHash}, got ${crypto.createHash("sha256").update(binaryData).digest("hex")}`
      );
    }

    // Cache the binary
    console.error(`[mcp-lens] Caching binary to ${cachedBinaryPath}...`);
    fs.writeFileSync(cachedBinaryPath, binaryData);
    fs.chmodSync(cachedBinaryPath, 0o755);

    // Execute the binary
    console.error(`[mcp-lens] Executing mcp-lens...`);
    execBinary(cachedBinaryPath);
  } catch (error) {
    console.error(
      `[mcp-lens] Error: ${error instanceof Error ? error.message : String(error)}`
    );
    process.exit(1);
  }
}

// Execute binary with inherited stdio and forward arguments
function execBinary(binaryPath: string): void {
  const args = process.argv.slice(2);

  const child = spawn(binaryPath, args, {
    stdio: "inherit",
    shell: false,
  });

  child.on("exit", (code) => {
    process.exit(code ?? 0);
  });

  child.on("error", (error) => {
    console.error(`[mcp-lens] Failed to execute binary: ${error.message}`);
    process.exit(1);
  });
}

// Run the main function
main();
