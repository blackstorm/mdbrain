import { TFile } from "obsidian";
import { describe, expect, test } from "vitest";
import {
  filterIgnoredFiles,
  isPathIgnored,
  normalizeIgnoreRules,
  normalizeVaultPath,
} from "./ignore-paths";

describe("ignore paths", () => {
  test("normalizes vault paths", () => {
    expect(normalizeVaultPath("./notes\\private.md")).toBe("notes/private.md");
    expect(normalizeVaultPath("//assets///img.png")).toBe("assets/img.png");
  });

  test("normalizes and deduplicates ignore rules", () => {
    const rules = normalizeIgnoreRules([
      "  Templates/  ",
      "./Templates/",
      "notes\\private.md",
      "",
      "  ",
    ]);

    expect(rules).toEqual(["Templates/", "notes/private.md"]);
  });

  test("matches directory prefixes", () => {
    const rules = ["Templates/"];

    expect(isPathIgnored("Templates/daily/2026-03-02.md", rules)).toBe(true);
    expect(isPathIgnored("Template/daily.md", rules)).toBe(false);
  });

  test("matches exact file rules", () => {
    const rules = ["notes/private.md"];

    expect(isPathIgnored("notes/private.md", rules)).toBe(true);
    expect(isPathIgnored("notes/private.md.bak", rules)).toBe(false);
  });

  test("treats non-suffixed rule as path prefix with boundary", () => {
    const rules = ["Archive"];

    expect(isPathIgnored("Archive/old.md", rules)).toBe(true);
    expect(isPathIgnored("Archive", rules)).toBe(true);
    expect(isPathIgnored("Archived/old.md", rules)).toBe(false);
  });

  test("filters ignored files", () => {
    const files = [
      new TFile("notes/a.md"),
      new TFile("Templates/private.md"),
      new TFile("assets/img.png", "img", "png"),
    ];

    const kept = filterIgnoredFiles(files, ["Templates/", "assets/img.png"]);

    expect(kept.map((file) => file.path)).toEqual(["notes/a.md"]);
  });
});
