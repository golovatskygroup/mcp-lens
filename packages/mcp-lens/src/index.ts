#!/usr/bin/env node

import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import * as crypto from "crypto";
import { spawn } from "child_process";
import * as https from "https";
import * as zlib from "zlib";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Platform detection and mapping
interface PlatformInfo {
  os: "darwin" | "linux" | "windows";
  arch: "arm64" | "amd64";
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

  let archType: "arm64" | "amd64";
  if (arch === "arm64") {
    archType = "arm64";
  } else if (arch === "x64") {
    archType = "amd64";
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

// Get cache directory (OS-specific)
function getCacheDir(): string {
  if (process.platform === "darwin") {
    const dir = path.join(os.homedir(), "Library", "Caches", "mcp-lens");
    fs.mkdirSync(dir, { recursive: true });
    return dir;
  }

  if (process.platform === "win32") {
    const base = process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local");
    const dir = path.join(base, "mcp-lens", "cache");
    fs.mkdirSync(dir, { recursive: true });
    return dir;
  }

  // linux/unix
  const xdg = process.env.XDG_CACHE_HOME;
  const dir = xdg ? path.join(xdg, "mcp-lens") : path.join(os.homedir(), ".cache", "mcp-lens");
  fs.mkdirSync(dir, { recursive: true });
  return dir;
}

// Download file from URL
function downloadFile(url: string): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];

    https
      .get(url, (response) => {
        if (response.statusCode === 301 || response.statusCode === 302) {
          if (response.headers.location) {
            downloadFile(response.headers.location).then(resolve).catch(reject);
            return;
          }
        }

        if (response.statusCode !== 200) {
          reject(new Error(`Failed to download: ${url} (${response.statusCode})`));
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

function pickChecksumForAsset(checksums: Map<string, string>, assetName: string): string {
  const direct = checksums.get(assetName);
  if (direct) return direct;

  // checksums.txt may prefix files with "./"
  const withDot = checksums.get(`./${assetName}`);
  if (withDot) return withDot;

  throw new Error(`Asset ${assetName} not found in checksums.txt`);
}

// Map to GitHub Release asset filename
function getAssetFilename(releaseTag: string, platform: PlatformInfo): string {
  const ext = platform.os === "windows" ? ".zip" : ".tar.gz";
  // GoReleaser asset template:
  // mcp-lens_<version>_<os>_<arch>.(tar.gz|zip)
  // where version is WITHOUT leading 'v'.
  const version = releaseTag.startsWith("v") ? releaseTag.slice(1) : releaseTag;
  return `mcp-lens_${version}_${platform.os}_${platform.arch}${ext}`;
}

function getBinaryName(platform: PlatformInfo): string {
  return platform.os === "windows" ? "mcp-lens.exe" : "mcp-lens";
}

function getCachedBinaryFilename(releaseTag: string, platform: PlatformInfo): string {
  const suffix = platform.os === "windows" ? ".exe" : "";
  // cache per version+platform to avoid collisions
  return `mcp-lens_${releaseTag}_${platform.os}_${platform.arch}${suffix}`;
}

function listFilesInTarGz(data: Buffer): string[] {
  const decompressed = zlib.gunzipSync(data);
  const files: string[] = [];

  // Minimal tar reader: 512-byte headers. File name is first 100 bytes.
  let offset = 0;
  while (offset + 512 <= decompressed.length) {
    const header = decompressed.subarray(offset, offset + 512);
    const isZeroBlock = header.every((b) => b === 0);
    if (isZeroBlock) break;

    const nameRaw = header.subarray(0, 100);
    const name = nameRaw.toString("utf8").replace(/\0.*$/, "").trim();

    if (name) files.push(name);

    // size field is octal at offset 124 length 12
    const sizeRaw = header.subarray(124, 136).toString("utf8").replace(/\0.*$/, "").trim();
    const size = parseInt(sizeRaw, 8) || 0;

    const dataStart = offset + 512;
    const dataEnd = dataStart + size;

    offset = dataStart + Math.ceil(size / 512) * 512;
    if (dataEnd > decompressed.length) break;
  }

  return files;
}

function extractFileFromTarGz(data: Buffer, filename: string): Buffer {
  const decompressed = zlib.gunzipSync(data);

  let offset = 0;
  while (offset + 512 <= decompressed.length) {
    const header = decompressed.subarray(offset, offset + 512);
    const isZeroBlock = header.every((b) => b === 0);
    if (isZeroBlock) break;

    const name = header.subarray(0, 100).toString("utf8").replace(/\0.*$/, "").trim();
    const sizeRaw = header.subarray(124, 136).toString("utf8").replace(/\0.*$/, "").trim();
    const size = parseInt(sizeRaw, 8) || 0;

    const dataStart = offset + 512;
    const dataEnd = dataStart + size;

    if (name === filename) {
      return decompressed.subarray(dataStart, dataEnd);
    }

    offset = dataStart + Math.ceil(size / 512) * 512;
    if (dataEnd > decompressed.length) break;
  }

  throw new Error(`File not found in tar.gz: ${filename}`);
}

function extractFileFromZip(zipData: Buffer, filename: string): Buffer {
  // Minimal ZIP extractor based on central directory.
  // Supports stored (0) and deflated (8).

  const EOCD_SIG = 0x06054b50;
  const CD_SIG = 0x02014b50;
  const LFH_SIG = 0x04034b50;

  const maxComment = 0x10000; // per ZIP spec
  const minEOCD = 22;
  const start = Math.max(0, zipData.length - minEOCD - maxComment);

  let eocdOffset = -1;
  for (let i = zipData.length - minEOCD; i >= start; i--) {
    if (zipData.readUInt32LE(i) === EOCD_SIG) {
      eocdOffset = i;
      break;
    }
  }
  if (eocdOffset < 0) throw new Error("Invalid zip: EOCD not found");

  const totalEntries = zipData.readUInt16LE(eocdOffset + 10);
  const cdSize = zipData.readUInt32LE(eocdOffset + 12);
  const cdOffset = zipData.readUInt32LE(eocdOffset + 16);

  let ptr = cdOffset;
  const cdEnd = cdOffset + cdSize;

  for (let entry = 0; entry < totalEntries && ptr + 46 <= cdEnd; entry++) {
    const sig = zipData.readUInt32LE(ptr);
    if (sig !== CD_SIG) break;

    const compression = zipData.readUInt16LE(ptr + 10);
    const compressedSize = zipData.readUInt32LE(ptr + 20);
    const uncompressedSize = zipData.readUInt32LE(ptr + 24);
    const fileNameLen = zipData.readUInt16LE(ptr + 28);
    const extraLen = zipData.readUInt16LE(ptr + 30);
    const commentLen = zipData.readUInt16LE(ptr + 32);
    const localHeaderOffset = zipData.readUInt32LE(ptr + 42);

    const nameStart = ptr + 46;
    const nameEnd = nameStart + fileNameLen;
    const name = zipData.subarray(nameStart, nameEnd).toString("utf8");

    ptr = nameEnd + extraLen + commentLen;

    if (name !== filename) continue;

    // Local file header
    if (zipData.readUInt32LE(localHeaderOffset) !== LFH_SIG) {
      throw new Error("Invalid zip: local file header not found");
    }

    const lfhCompression = zipData.readUInt16LE(localHeaderOffset + 8);
    const lfhNameLen = zipData.readUInt16LE(localHeaderOffset + 26);
    const lfhExtraLen = zipData.readUInt16LE(localHeaderOffset + 28);

    const dataStart = localHeaderOffset + 30 + lfhNameLen + lfhExtraLen;
    const dataEnd = dataStart + compressedSize;
    if (dataEnd > zipData.length) throw new Error("Invalid zip: truncated file data");

    const compressed = zipData.subarray(dataStart, dataEnd);

    if (lfhCompression !== compression) {
      throw new Error("Invalid zip: compression mismatch");
    }

    if (compression === 0) {
      // stored
      return compressed;
    }

    if (compression === 8) {
      const inflated = zlib.inflateRawSync(compressed);
      if (uncompressedSize !== 0 && inflated.length !== uncompressedSize) {
        // best-effort check (size can be 0 if ZIP64, not expected here)
        throw new Error("Invalid zip: uncompressed size mismatch");
      }
      return inflated;
    }

    throw new Error(`Unsupported zip compression method: ${compression}`);
  }

  throw new Error(`File not found in zip: ${filename}`);
}

function extractBinaryFromArchive(platform: PlatformInfo, archiveData: Buffer): Buffer {
  const binName = getBinaryName(platform);

  if (platform.os === "windows") {
    // zip
    const candidates = [binName, `./${binName}`];
    for (const c of candidates) {
      try {
        return extractFileFromZip(archiveData, c);
      } catch {
        // try next
      }
    }
    throw new Error(`Binary '${binName}' not found in zip archive`);
  }

  // tar.gz
  const files = listFilesInTarGz(archiveData);

  // Try exact match first, then try with leading './'
  const candidates = [binName, `./${binName}`];
  const match = candidates.find((c) => files.includes(c));
  if (!match) {
    throw new Error(
      `Binary '${binName}' not found in archive. Found: ${files.slice(0, 10).join(", ")}`
    );
  }

  return extractFileFromTarGz(archiveData, match);
}

// Main function
async function main(): Promise<void> {
  try {
    const platform = detectPlatform();
    console.error(`[mcp-lens] Detected platform: ${platform.os} ${platform.arch}`);

    const releaseTag = getReleaseTag();
    console.error(`[mcp-lens] Using release tag: ${releaseTag}`);

    const cacheDir = getCacheDir();
    const cachedBinaryFilename = getCachedBinaryFilename(releaseTag, platform);
    const cachedBinaryPath = path.join(cacheDir, cachedBinaryFilename);

    if (fs.existsSync(cachedBinaryPath)) {
      console.error(`[mcp-lens] Using cached binary: ${cachedBinaryPath}`);
      execBinary(cachedBinaryPath);
      return;
    }

    const assetFilename = getAssetFilename(releaseTag, platform);

    const checksumUrl = `https://github.com/golovatskygroup/mcp-lens/releases/download/${releaseTag}/checksums.txt`;
    const archiveUrl = `https://github.com/golovatskygroup/mcp-lens/releases/download/${releaseTag}/${assetFilename}`;

    console.error(`[mcp-lens] Downloading checksums.txt...`);
    const checksumContent = await downloadFile(checksumUrl);
    const checksums = parseChecksums(checksumContent.toString("utf-8"));

    const expectedHash = pickChecksumForAsset(checksums, assetFilename);

    console.error(`[mcp-lens] Downloading release asset: ${assetFilename}...`);
    const archiveData = await downloadFile(archiveUrl);

    console.error(`[mcp-lens] Verifying checksum...`);
    if (!verifySha256(archiveData, expectedHash)) {
      const got = crypto.createHash("sha256").update(archiveData).digest("hex");
      throw new Error(
        `Checksum verification failed for ${assetFilename}. Expected ${expectedHash}, got ${got}`
      );
    }

    console.error(`[mcp-lens] Extracting binary from archive...`);
    const binaryData = extractBinaryFromArchive(platform, archiveData);

    console.error(`[mcp-lens] Caching binary to ${cachedBinaryPath}...`);
    fs.writeFileSync(cachedBinaryPath, binaryData);
    if (platform.os !== "windows") {
      fs.chmodSync(cachedBinaryPath, 0o755);
    }

    console.error(`[mcp-lens] Executing mcp-lens...`);
    execBinary(cachedBinaryPath);
  } catch (error) {
    console.error(`[mcp-lens] Error: ${error instanceof Error ? error.message : String(error)}`);
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

main();
