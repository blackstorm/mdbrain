import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const rootDir = process.cwd();
const templatesDir = path.join(rootDir, 'resources', 'templates');
const resourcesDir = path.join(rootDir, 'resources');
const lucideSourceDir = path.join(templatesDir, 'lucide');

const generatedResourcesDir = path.join(rootDir, 'target', 'generated-resources');
const generatedPublicsDir = path.join(generatedResourcesDir, 'publics');
const generatedLucideDir = path.join(generatedResourcesDir, 'templates', 'lucide');
const generatedAssetManifestFile = path.join(generatedPublicsDir, 'asset-manifest.json');

const assetUrlPattern = /\{\{\s*(['"])(\/publics\/[^'"\s]+)\1\s*\|\s*asset_url\s*\}\}/g;
const lucideCallPattern = /lucide_icon\s*\(\s*"([a-z0-9-]+)"/g;
const lucideArgPattern = /(?:^|\s)(?:icon|cta-icon)\s*=\s*"([a-z0-9-]+)"/g;

const legacyAliases = {
  'check-circle': 'circle-check-big',
  'alert-circle': 'circle-alert',
  'alert-triangle': 'triangle-alert',
  'more-vertical': 'ellipsis-vertical'
};

function listHtmlFiles(dir) {
  const results = [];
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const filePath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...listHtmlFiles(filePath));
      continue;
    }
    if (entry.isFile() && entry.name.endsWith('.html')) {
      results.push(filePath);
    }
  }
  return results;
}

function shortSha256(filePath) {
  const content = fs.readFileSync(filePath);
  return crypto.createHash('sha256').update(content).digest('hex').slice(0, 12);
}

function splitBaseAndSuffix(assetPath) {
  const match = String(assetPath).match(/^([^?#]*)(\?[^#]*)?(#.*)?$/);
  if (!match) {
    return [String(assetPath), ''];
  }
  const base = match[1];
  const suffix = `${match[2] || ''}${match[3] || ''}`;
  return [base, suffix];
}

function fingerprintedPath(assetPath, hash) {
  const normalized = String(assetPath);
  const lastSlash = normalized.lastIndexOf('/');
  const dir = lastSlash >= 0 ? normalized.slice(0, lastSlash + 1) : '';
  const fileName = lastSlash >= 0 ? normalized.slice(lastSlash + 1) : normalized;
  const lastDot = fileName.lastIndexOf('.');

  if (lastDot > 0) {
    return `${dir}${fileName.slice(0, lastDot)}.${hash}${fileName.slice(lastDot)}`;
  }
  return `${dir}${fileName}.${hash}`;
}

function extractMatches(text, pattern) {
  const values = [];
  let match;
  while ((match = pattern.exec(text)) !== null) {
    values.push(match[1]);
  }
  return values;
}

function generateAssetManifest(templateFiles) {
  const referencedAssets = new Set();

  for (const filePath of templateFiles) {
    const content = fs.readFileSync(filePath, 'utf8');
    let match;
    while ((match = assetUrlPattern.exec(content)) !== null) {
      referencedAssets.add(match[2]);
    }
  }

  const sortedAssets = Array.from(referencedAssets).sort();
  const manifest = {};

  for (const assetPath of sortedAssets) {
    const [base] = splitBaseAndSuffix(assetPath);
    const sourcePath = path.join(resourcesDir, base.replace(/^\//, ''));
    if (!fs.existsSync(sourcePath)) {
      throw new Error(`Asset referenced by template does not exist: ${assetPath}`);
    }

    const hash = shortSha256(sourcePath);
    const fingerprinted = fingerprintedPath(base, hash);
    const targetPath = path.join(generatedResourcesDir, fingerprinted.replace(/^\//, ''));

    fs.mkdirSync(path.dirname(targetPath), { recursive: true });
    fs.copyFileSync(sourcePath, targetPath);
    manifest[base] = fingerprinted;
  }

  fs.mkdirSync(generatedPublicsDir, { recursive: true });
  fs.writeFileSync(generatedAssetManifestFile, JSON.stringify(manifest));
  return Object.keys(manifest).length;
}

function resolveLucideSource(iconName) {
  const directPath = path.join(lucideSourceDir, `${iconName}.svg`);
  if (fs.existsSync(directPath)) {
    return { sourcePath: directPath, targetName: iconName };
  }

  const aliasName = legacyAliases[iconName];
  if (!aliasName) {
    return null;
  }

  const aliasPath = path.join(lucideSourceDir, `${aliasName}.svg`);
  if (!fs.existsSync(aliasPath)) {
    return null;
  }

  return { sourcePath: aliasPath, targetName: aliasName };
}

function generateLucideSubset(templateFiles) {
  const requestedIcons = new Set(['plus']);

  for (const filePath of templateFiles) {
    const content = fs.readFileSync(filePath, 'utf8');
    for (const iconName of extractMatches(content, lucideCallPattern)) {
      requestedIcons.add(iconName);
    }
    for (const iconName of extractMatches(content, lucideArgPattern)) {
      requestedIcons.add(iconName);
    }
  }

  const resolved = new Map();
  const missing = [];

  for (const iconName of requestedIcons) {
    const icon = resolveLucideSource(iconName);
    if (!icon) {
      missing.push(iconName);
      continue;
    }
    resolved.set(icon.targetName, icon.sourcePath);
  }

  if (missing.length > 0) {
    missing.sort();
    throw new Error(`Missing lucide icon files: ${missing.join(', ')}`);
  }

  fs.mkdirSync(generatedLucideDir, { recursive: true });
  for (const [targetName, sourcePath] of resolved.entries()) {
    const outputPath = path.join(generatedLucideDir, `${targetName}.svg`);
    fs.copyFileSync(sourcePath, outputPath);
  }

  return resolved.size;
}

function main() {
  fs.rmSync(generatedResourcesDir, { recursive: true, force: true });

  const templateFiles = listHtmlFiles(templatesDir);
  const assetCount = generateAssetManifest(templateFiles);
  const iconCount = generateLucideSubset(templateFiles);

  console.log(`Generated asset manifest entries: ${assetCount}`);
  console.log(`Generated lucide icon subset: ${iconCount} icons`);
}

main();
