import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { generateMarkDown, getGitDiff, getRepoConfig, parseCommits } from "changelogen";
import { assetNames, platforms } from "./assets.mjs";

const stableTag = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const types = Object.fromEntries(Object.entries({
  feat: "Enhancements",
  perf: "Performance",
  fix: "Fixes",
  refactor: "Refactors",
  docs: "Documentation",
  build: "Build",
  types: "Types",
  chore: "Chore",
  examples: "Examples",
  test: "Tests",
  style: "Styles",
  ci: "CI",
}).map(([type, title]) => [type, { title }]));

function contributors(raw) {
  const people = new Map();
  const authors = raw.flatMap((commit) => [
    commit.author,
    ...Array.from(commit.body.matchAll(/^Co-authored-by:\s*(.+?)\s*<([^>]+)>\s*$/gim),
      (match) => ({ name: match[1], email: match[2] })),
  ]);
  for (const author of authors) {
    const name = author?.name?.trim();
    if (!name || name.includes("[bot]")) continue;
    const email = author.email?.trim().toLowerCase() ?? "";
    const username = email.match(/(?:^|\+)([a-z\d-]+)@users\.noreply\.github\.com$/i)?.[1];
    const key = username ? `github:${username}` : email || name;
    const label = name.replace(/[\\`*_[\]<>]/g, "\\$&");
    people.set(key, username
      ? `- ${label} ([@${username}](https://github.com/${username}))`
      : `- ${label}`);
  }
  return [...people.values()];
}

export async function releaseNotes(tag, cwd = process.cwd(), repository = process.env.GITHUB_REPOSITORY || "merc-hecl/BidBrawl") {
  if (!stableTag.test(tag ?? "")) throw new Error("Expected a stable release tag, e.g. v0.1.4");
  const git = (...args) => execFileSync("git", args, {
    cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
  }).trim();
  if (git("rev-parse", "--is-shallow-repository") === "true")
    throw new Error("Release notes require full Git history (fetch-depth: 0)");

  const pkg = JSON.parse(git("show", `${tag}:mygo.json`));
  if (tag !== `v${pkg.version}`) throw new Error("Release tag must match mygo.json version");
  // 只取当前标签的祖先版本；忽略预发布标签及同一提交上的更高版本。
  const tags = git("tag", "--merged", tag, "--sort=-version:refname", "--list", "v*")
    .split("\n").filter((entry) => stableTag.test(entry));
  const previous = tags[tags.indexOf(tag) + 1] ?? "";
  const config = {
    from: previous,
    to: tag,
    newVersion: pkg.version,
    types,
    scopeMap: {},
    repo: getRepoConfig(`https://github.com/${repository}`),
    templates: { tagBody: "v{{newVersion}}" },
    noAuthors: true,
  };
  const raw = await getGitDiff(previous || undefined, tag, cwd);
  const parsed = parseCommits(raw, config);
  const commits = parsed
    .map((commit) => ({ ...commit, type: commit.type.toLowerCase(), author: undefined }))
    .filter((commit) => types[commit.type]);
  // 本地收集作者和 Co-authored-by，避免依赖 changelogen 的远端作者查询。
  const markdown = await generateMarkDown(commits, config);
  const body = markdown
    .replace(/^## [^\n]+\n*/, "")
    .replace(/^#### ⚠️ Breaking Changes$/m, "#### Breaking Changes")
    .trim();
  const people = contributors(raw);
  const labels = ["Windows x64", "Linux x64", "Linux ARM64", "macOS Intel", "macOS Apple Silicon"];
  const downloadURL = `https://github.com/${repository}/releases/download/${tag}/`;
  const link = (name, label) => `[${label}](${downloadURL}${encodeURIComponent(name)})`;
  const downloads = [
    "### Downloads",
    "",
    "Desktop installers include the host, WebUI, and standalone CLI. CLI archives also support headless hosting.",
    "",
    "| Platform | Desktop + CLI | CLI only |\n| --- | --- | --- |",
    ...platforms.map((platform, index) => {
      const { installer, archive } = assetNames(pkg.version, platform);
      return `| ${labels[index]} | ${link(installer, installer.endsWith('.exe') ? 'Setup' : installer.endsWith('.deb') ? 'DEB' : 'PKG')} | ${link(archive, archive.endsWith('.zip') ? 'ZIP' : 'tar.gz')} |`;
    }),
  ].join("\n");
  const installation = [
    `Checksums: ${link("SHA256SUMS", "SHA256SUMS")} (SHA-256).`,
    "Windows installers are not Authenticode-signed and require WebView2 Runtime. macOS packages are not Developer ID-signed or notarized; Gatekeeper may block installation. Linux desktop packages require GTK 3 and WebKitGTK 4.1.",
    "Stop the running host before upgrading. Only the current database format is supported; back up your data before upgrading.",
  ].join("\n\n");
  return [body, ...(people.length ? ["### Contributors", people.join("\n")] : []), downloads, installation]
    .filter(Boolean).join("\n\n") + "\n";
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [tag, output, ...extra] = process.argv.slice(2);
  if (!output || extra.length) throw new Error("Usage: node .github/release/notes.mjs <tag> <output.md>");
  const markdown = await releaseNotes(tag);
  mkdirSync(dirname(resolve(output)), { recursive: true });
  writeFileSync(output, markdown, "utf8");
}
