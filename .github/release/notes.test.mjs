import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath, pathToFileURL } from "node:url";
import { releaseNotes as generateNotes } from "./notes.mjs";

const releaseNotes = (tag, cwd) => generateNotes(tag, cwd, "merc-hecl/BidBrawl");

const script = fileURLToPath(new URL("./notes.mjs", import.meta.url));

function repository(t) {
  const dir = mkdtempSync(join(tmpdir(), "bidbrawl-release-notes-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const git = (...args) => execFileSync("git", args, {
    cwd: dir, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
  }).trim();
  git("init", "-b", "main");
  git("config", "user.name", "Test Author");
  git("config", "user.email", "123+test-author@users.noreply.github.com");
  git("config", "commit.gpgsign", "false");
  const pkg = { version: "0.1.0", repository: { url: "https://github.com/merc-hecl/BidBrawl.git" } };
  let count = 0;
  const commit = (message, version = pkg.version, extra = []) => {
    pkg.version = version;
    writeFileSync(join(dir, "mygo.json"), JSON.stringify(pkg));
    writeFileSync(join(dir, "content.txt"), String(++count));
    git("add", "--all");
    git("commit", "--no-verify", "-m", message, ...extra);
    return git("rev-parse", "--short", "HEAD");
  };
  commit("feat: initial feature");
  git("tag", "v0.1.0");
  return { dir, git, commit };
}

test("notes classify only the requested version range and link commits", async (t) => {
  const { dir, git, commit } = repository(t);
  const feat = commit("feat(settings): remember host settings");
  const fix = commit("fix: restore room selection");
  commit("refactor: simplify settings");
  commit("chore: release 0.1.1", "0.1.1");
  git("tag", "-a", "v0.1.1", "-m", "0.1.1");
  commit("feat: future feature", "0.1.2");
  git("tag", "v0.1.2");
  const notes = await releaseNotes("v0.1.1", dir);
  assert.match(notes, /compare\/v0\.1\.0\.\.\.v0\.1\.1/);
  for (const title of ["Enhancements", "Fixes", "Refactors", "Chore", "Contributors"])
    assert.ok(notes.includes(`### ${title}\n`), title);
  assert.ok(notes.includes(`**settings:** Remember host settings ([${feat}](https://github.com/merc-hecl/BidBrawl/commit/${feat}))`));
  assert.ok(notes.includes(`/commit/${fix}`));
  assert.doesNotMatch(notes, /Initial feature|Future feature|^## v/m);
  assert.ok(!notes.split("\n").filter((line) => line.startsWith("#")).some((line) => /\p{Extended_Pictographic}/u.test(line)));
});

test("first release includes its root commit", async (t) => {
  const { dir } = repository(t);
  const notes = await releaseNotes("v0.1.0", dir);
  assert.match(notes, /### Enhancements\n/);
  assert.match(notes, /Initial feature/);
  assert.doesNotMatch(notes, /compare\//);
  assert.equal((notes.match(/releases\/download\/v0\.1\.0\//g) || []).length, 11);
  assert.match(notes, /BidBrawl%20Setup%200.1.0%20amd64.exe/);
  assert.match(notes, /not Developer ID-signed or notarized/);
});

test("previous tag ignores prereleases and tags on unmerged branches", async (t) => {
  const { dir, git, commit } = repository(t);
  git("checkout", "-b", "unmerged");
  commit("feat: unrelated feature", "0.9.0");
  git("tag", "v0.9.0");
  git("checkout", "main");
  commit("feat: preview feature", "0.1.1-rc.1");
  git("tag", "v0.1.1-rc.1");
  commit("fix: finalize preview", "0.1.1");
  git("tag", "v0.1.1");
  const notes = await releaseNotes("v0.1.1", dir);
  assert.match(notes, /compare\/v0\.1\.0\.\.\.v0\.1\.1/);
  assert.match(notes, /Preview feature/);
  assert.doesNotMatch(notes, /Unrelated feature/);
});

test("contributors include coauthors and unclassified commits, without duplicates or bots", async (t) => {
  const { dir, git, commit } = repository(t);
  commit("feat: coauthored feature", "0.1.0", ["-m", "Co-authored-by: Partner <partner@example.test>"]);
  commit("fix: another contribution");
  commit("Unclassified contribution", "0.1.1", [
    "--author=Another Author <another@example.test>",
    "-m", "Co-authored-by: Another Partner <another-partner@example.test>",
  ]);
  commit("ci: bot automation", "0.1.1", ["--author=github-actions[bot] <bot@example.test>"]);
  git("tag", "v0.1.1");
  const notes = await releaseNotes("v0.1.1", dir);
  const people = notes.split("### Contributors\n")[1];
  assert.equal(people.match(/\[@test-author\]/g)?.length, 1);
  assert.match(people, /https:\/\/github\.com\/test-author/);
  assert.match(people, /- Partner/);
  assert.match(people, /- Another Author/);
  assert.match(people, /- Another Partner/);
  assert.doesNotMatch(people, /github-actions|@example\.test/);
});

test("breaking changes have a plain English heading", async (t) => {
  const { dir, git, commit } = repository(t);
  commit("feat(api)!: remove legacy format", "0.2.0");
  git("tag", "v0.2.0");
  const notes = await releaseNotes("v0.2.0", dir);
  assert.match(notes, /^#### Breaking Changes$/m);
  assert.doesNotMatch(notes, /^#+ .*\p{Extended_Pictographic}/mu);
});

test("invalid or mismatched tags and shallow history fail explicitly", async (t) => {
  const { dir, git, commit } = repository(t);
  await assert.rejects(releaseNotes("v0.1.1-rc.1", dir), /stable release tag/);
  git("tag", "v0.1.2");
  await assert.rejects(releaseNotes("v0.1.2", dir), /must match/);
  commit("fix: shallow test", "0.1.1");
  git("tag", "v0.1.1");
  const clone = join(dir, "shallow");
  git("clone", "--depth=1", "--branch=main", pathToFileURL(dir).href, clone);
  await assert.rejects(releaseNotes("v0.1.1", clone), /full Git history/);
});

test("CLI writes repeatable Markdown without changing versions or Git refs", (t) => {
  const { dir, git } = repository(t);
  const refs = git("show-ref");
  const packageBefore = readFileSync(join(dir, "mygo.json"), "utf8");
  const output = join(dir, "notes", "release.md");
  const run = () => execFileSync(process.execPath, [script, "v0.1.0", output], {
    cwd: dir, stdio: ["ignore", "pipe", "pipe"],
  });
  run();
  const first = readFileSync(output, "utf8");
  run();
  assert.equal(readFileSync(output, "utf8"), first);
  assert.equal(git("show-ref"), refs);
  assert.equal(readFileSync(join(dir, "mygo.json"), "utf8"), packageBefore);
});
