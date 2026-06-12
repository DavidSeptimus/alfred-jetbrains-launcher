import { cpSync, mkdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const here = dirname(fileURLToPath(import.meta.url));
const extensionRoot = resolve(here, "..");
const repoRoot = resolve(extensionRoot, "..", "..");
const assetsDir = join(extensionRoot, "assets");
const binDir = join(assetsDir, "bin");
const tmpDir = join(extensionRoot, ".backend-tmp");
const out = join(binDir, "jb");

function run(cmd, args, options = {}) {
  const result = spawnSync(cmd, args, {
    cwd: repoRoot,
    stdio: "inherit",
    env: {
      ...process.env,
      CGO_ENABLED: "0",
      GOOS: "darwin",
      ...options.env,
    },
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

mkdirSync(binDir, { recursive: true });
mkdirSync(tmpDir, { recursive: true });

const ldflags = "-s -w -X main.version=dev -X main.channel=dev";
const arm64 = join(tmpDir, "jb-arm64");
const amd64 = join(tmpDir, "jb-amd64");

run("go", ["build", "-ldflags", ldflags, "-o", arm64, "./cmd/jb"], {
  env: { GOARCH: "arm64" },
});
run("go", ["build", "-ldflags", ldflags, "-o", amd64, "./cmd/jb"], {
  env: { GOARCH: "amd64" },
});
run("lipo", ["-create", "-output", out, arm64, amd64], { env: {} });
run("codesign", ["--force", "-s", "-", out], { env: {} });

rmSync(tmpDir, { recursive: true, force: true });
rmSync(join(assetsDir, "icons"), { recursive: true, force: true });
cpSync(join(repoRoot, "assets", "icons"), join(assetsDir, "icons"), {
  recursive: true,
});

console.log(`staged ${out}`);
