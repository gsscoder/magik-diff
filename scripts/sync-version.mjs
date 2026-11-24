// Copies the version from main.go (the single source of truth) into the other
// files that embed it. Plain text replacement keeps formatting and line endings
// byte-identical except for the version text.
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// Each pattern has two capture groups: the text before and after the version.
const targets = [
  { file: "wails.json", patterns: [/("productVersion"\s*:\s*")[^"]*(")/] },
  { file: "frontend/package.json", patterns: [/^(  "version": ")[^"]*(")/m] },
  {
    file: "frontend/package-lock.json",
    patterns: [
      /^(  "version": ")[^"]*(")/m,
      /(^    "": \{[\s\S]*?^      "version": ")[^"]*(")/m,
    ],
  },
  { file: "README.md", patterns: [/(badge\/version-)\d+\.\d+\.\d+(-blue)/] },
];

const mainSource = readFileSync(resolve(root, "main.go"), "utf8");
const version = /var version = "(\d+\.\d+\.\d+)"/.exec(mainSource)?.[1];
if (version === undefined) {
  console.error('main.go: no line matching `var version = "X.Y.Z"`');
  process.exit(1);
}

const changes = [];
const missing = [];
for (const { file, patterns } of targets) {
  const original = readFileSync(resolve(root, file), "utf8");
  let updated = original;
  for (const pattern of patterns) {
    if (!pattern.test(updated)) {
      missing.push(`${file}: pattern ${pattern} not found`);
      continue;
    }
    updated = updated.replace(pattern, (_, before, after) => before + version + after);
  }
  if (updated !== original) {
    changes.push({ file, updated });
  }
}

if (missing.length > 0) {
  console.error(missing.join("\n"));
  process.exit(1);
}

for (const { file, updated } of changes) {
  writeFileSync(resolve(root, file), updated);
  console.log(`${file}: set to ${version}`);
}
if (changes.length === 0) {
  console.log(`already in sync (${version})`);
}
