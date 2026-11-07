import { router, json, error, db, requireAuth, storage, secrets } from '@appdeploy/sdk';

type ProjectFile = {
  name: string;
  size: number;
  kind: string;
  uploaded: boolean;
  storagePath?: string;
  contentType?: string;
};

type ProjectAnalysis = {
  framework: string;
  packageManager: string;
  deploymentTarget: string;
  entrypoint: string;
  confidence: 'high' | 'medium' | 'low';
  warnings: string[];
  checkedAt: string;
};

type ProjectRecord = {
  ownerId: string;
  name: string;
  framework: string;
  status: 'draft';
  files: ProjectFile[];
  fileCount: number;
  totalBytes: number;
  createdAt: string;
  analysis?: ProjectAnalysis;
  engineProjectId?: string;
  slug?: string;
  sourceUploaded?: boolean;
  deploymentStatus?: string;
  buildJobId?: string;
  deploymentId?: string;
};

function tableFor(userId: string) {
  const safeId = userId.replace(/[^A-Za-z0-9]/g, '');
  if (!safeId) throw new Error('Invalid authenticated account identifier.');
  return 'projects_' + safeId;
}

function validName(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length >= 2 && value.trim().length <= 48;
}

function safeFileName(value: string) {
  return value.length <= 180 &&
    !value.includes('/') &&
    !value.includes('\\') &&
    !value.includes('..') &&
    !/[\u0000-\u001f]/.test(value);
}

function contentTypeFor(name: string, supplied: unknown) {
  const extension = name.toLowerCase().split('.').pop() || '';
  const known: Record<string, string> = {
    zip: 'application/zip',
    html: 'text/html',
    htm: 'text/html',
    jsx: 'text/javascript',
    js: 'text/javascript',
    tsx: 'text/plain',
    ts: 'text/plain',
    css: 'text/css',
    vue: 'text/plain',
    json: 'application/json'
  };
  const expected = known[extension];
  if (!expected) return null;
  if (typeof supplied !== 'string' || supplied.length > 120) return expected;
  return expected;
}

function analyzeProject(files: ProjectFile[]): ProjectAnalysis {
  const names = files.map(file => file.name.toLowerCase());
  const has = (name: string) => names.includes(name);
  const hasPrefix = (prefix: string) => names.some(name => name.startsWith(prefix));
  const isArchive = names.some(name => name.endsWith('.zip'));
  const hasPackage = has('package.json');
  const warnings: string[] = [];

  if (isArchive) {
    return {
      framework: 'Archive pending inspection',
      packageManager: 'Not detected',
      deploymentTarget: 'Pending secure ZIP inspection',
      entrypoint: 'Not inspected',
      confidence: 'low',
      warnings: [
        'ZIP contents have not been extracted or inspected. Secure archive validation and isolated build workers are not connected.',
        'No build or public deployment has been started.'
      ],
      checkedAt: new Date().toISOString()
    };
  }

  let framework = 'Unknown frontend';
  let confidence: ProjectAnalysis['confidence'] = 'low';
  if (hasPrefix('next.config.')) {
    framework = 'Next.js';
    confidence = 'high';
  } else if (hasPrefix('vite.config.')) {
    framework = names.some(name => /\.(jsx|tsx)$/.test(name)) ? 'React + Vite' : 'Vite';
    confidence = 'high';
  } else if (names.some(name => name.endsWith('.vue'))) {
    framework = 'Vue';
    confidence = 'high';
  } else if (names.some(name => /\.(jsx|tsx)$/.test(name))) {
    framework = 'React';
    confidence = 'medium';
  } else if (names.some(name => /\.(html|htm)$/.test(name))) {
    framework = 'Static HTML';
    confidence = 'high';
  } else if (hasPackage) {
    framework = 'JavaScript / TypeScript project';
    confidence = 'low';
  }

  const packageManager = has('pnpm-lock.yaml') ? 'pnpm'
    : has('yarn.lock') ? 'Yarn'
      : has('bun.lockb') || has('bun.lock') ? 'Bun'
        : has('package-lock.json') ? 'npm'
          : hasPackage ? 'npm (default; lockfile not supplied)' : 'Not applicable';

  let entrypoint = 'Not detected';
  const entryCandidates = [
    'index.html',
    'src/main.tsx',
    'src/main.jsx',
    'src/main.ts',
    'src/main.js',
    'app/page.tsx',
    'app/page.jsx',
    'pages/index.tsx',
    'pages/index.jsx'
  ];
  entrypoint = entryCandidates.find(candidate => has(candidate)) || entrypoint;

  let deploymentTarget = 'Build configuration required';
  if (framework === 'Next.js') {
    deploymentTarget = 'Node.js runtime required (SSR-capable deployment)';
  } else if (framework === 'Static HTML' && !hasPackage) {
    deploymentTarget = 'Static file hosting';
  } else if (hasPackage) {
    deploymentTarget = 'Frontend build required; output directory not verified';
  }

  if (hasPackage && !has('package-lock.json') && !has('pnpm-lock.yaml') && !has('yarn.lock') && !has('bun.lock') && !has('bun.lockb')) {
    warnings.push('No supported dependency lockfile was supplied; reproducible dependency installation is not yet verified.');
  }
  if (hasPackage && !has('index.html') && !hasPrefix('next.config.') && !hasPrefix('vite.config.')) {
    warnings.push('No recognized root HTML or framework configuration file was found in the uploaded file list.');
  }
  if (entrypoint === 'Not detected') {
    warnings.push('A recognized application entrypoint was not found. Upload the project root structure for a more reliable assessment.');
  }
  if (!hasPackage && framework !== 'Static HTML') {
    warnings.push('package.json was not supplied. Framework and build commands cannot be confirmed from filenames alone.');
  }
  warnings.push('This is a metadata-based preflight only. Source code has not been executed and no build has been run.');

  return {
    framework,
    packageManager,
    deploymentTarget,
    entrypoint,
    confidence,
    warnings,
    checkedAt: new Date().toISOString()
  };
}

function publicProject(id: string, record: ProjectRecord) {
  const uploadedFiles = record.files.filter(file => file.uploaded);
  return {
    id,
    name: record.name,
    framework: record.analysis?.framework || record.framework,
    status: record.status,
    fileCount: record.fileCount,
    totalBytes: record.totalBytes,
    uploadedCount: uploadedFiles.length,
    uploadedBytes: uploadedFiles.reduce((sum, file) => sum + file.size, 0),
    createdAt: record.createdAt,
    analysis: record.analysis || null,
    engineProjectId: record.engineProjectId || null,
    slug: record.slug || null,
    sourceUploaded: record.sourceUploaded || false,
    deploymentStatus: record.deploymentStatus || null,
    buildJobId: record.buildJobId || null,
    deploymentId: record.deploymentId || null
  };
}

type EngineResult = { status: number; data: Record<string, unknown> };
async function engineSecrets() {
  const names = await secrets.listSecretNames();
  if (!names.includes('ENGINE_API_URL') || !names.includes('AUTH_HMAC_SECRET')) return null;
  const base = (await secrets.readSecret('ENGINE_API_URL')).trim();
  const secret = await secrets.readSecret('AUTH_HMAC_SECRET');
  if (secret.length < 32) throw new Error('AUTH_HMAC_SECRET must contain at least 32 characters.');
  const parsed = new URL(base);
  if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && ['localhost', '127.0.0.1'].includes(parsed.hostname))) throw new Error('ENGINE_API_URL must use HTTPS (HTTP is allowed only for localhost development).');
  if (parsed.username || parsed.password || parsed.search || parsed.hash || parsed.pathname !== '/') throw new Error('ENGINE_API_URL must be an origin URL without credentials, path, query or fragment.');
  return { baseUrl: parsed.origin, secret };
}
function bytesToHex(bytes: Uint8Array) { return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join(''); }
async function signedEngineRequest(userId: string, method: string, path: string, body?: unknown): Promise<EngineResult> {
  const config = await engineSecrets();
  if (!config) throw new Error('The Go hosting engine is not configured. Add ENGINE_API_URL and AUTH_HMAC_SECRET in secure app settings.');
  if (!path.startsWith('/api/v1/')) throw new Error('Invalid engine API path.');
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const nonceBytes = new Uint8Array(24); crypto.getRandomValues(nonceBytes);
  const nonce = bytesToHex(nonceBytes);
  const canonical = [timestamp, nonce, userId, method.toUpperCase(), path].join('\n');
  const key = await crypto.subtle.importKey('raw', new TextEncoder().encode(config.secret), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const digest = new Uint8Array(await crypto.subtle.sign('HMAC', key, new TextEncoder().encode(canonical)));
  const signature = bytesToHex(digest);
  const controller = new AbortController(); const timeout = setTimeout(() => controller.abort(), 15000);
  try {
    const response = await fetch(config.baseUrl + path, { method, headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-Authenticated-User': userId, 'X-Auth-Timestamp': timestamp, 'X-Auth-Nonce': nonce, 'X-Auth-Signature': signature }, body: body === undefined ? undefined : JSON.stringify(body), signal: controller.signal });
    const raw = await response.text(); let data: Record<string, unknown> = {};
    try { data = raw ? JSON.parse(raw) as Record<string, unknown> : {}; } catch { data = { error: 'The hosting engine returned an invalid response.' }; }
    return { status: response.status, data };
  } finally { clearTimeout(timeout); }
}
function engineResponse(result: EngineResult) { return json(result.data, result.status); }
async function engineReadiness() {
  const names = await secrets.listSecretNames();
  if (!names.includes('ENGINE_API_URL') || !names.includes('AUTH_HMAC_SECRET')) return { configured: false, ready: false, message: 'Add ENGINE_API_URL and AUTH_HMAC_SECRET using the secure configuration form.' };
  const config = await engineSecrets(); if (!config) return { configured: false, ready: false, message: 'Hosting engine configuration is incomplete.' };
  try {
    const response = await fetch(config.baseUrl + '/readyz', { signal: AbortSignal.timeout(5000) });
    const body = await response.json() as Record<string, unknown>;
    let publicBaseUrl: string | null = null;
    if (names.includes('PUBLIC_HOSTING_BASE_URL')) {
      const candidate = new URL((await secrets.readSecret('PUBLIC_HOSTING_BASE_URL')).trim());
      if (candidate.protocol === 'https:' && !candidate.username && !candidate.password && candidate.pathname === '/' && !candidate.search && !candidate.hash) publicBaseUrl = candidate.origin;
    }
    const checks = body.checks as Record<string, unknown> | undefined;
    const buildExecution = checks?.buildExecution || 'unknown';
    const message = !response.ok
      ? 'Go API is reachable but one or more dependencies are not ready.'
      : buildExecution === 'not-connected'
        ? 'Go API and storage are ready; isolated build execution is not connected yet.'
        : 'Go API and required infrastructure report ready.';
    return { configured: true, ready: response.ok && body.status === 'ready', message, buildExecution, publicBaseUrl };
  } catch { return { configured: true, ready: false, message: 'Could not reach the Go engine readiness endpoint.' }; }
}

export const handler = router({
  'GET /api/engine/status': [requireAuth(), async () => json(await engineReadiness())],
  'POST /api/engine/projects': [requireAuth(), async ({ user, body }) => {
    const payload = body as { name?: unknown }; if (!validName(payload?.name)) return error('Project name must contain 2–48 characters.', 400);
    try { return engineResponse(await signedEngineRequest(user!.userId, 'POST', '/api/v1/projects', { name: payload.name })); }
    catch (err) { return error(err instanceof Error ? err.message : 'Could not contact the hosting engine.', 503); }
  }],
  'POST /api/engine/projects/:projectId/source-upload': [requireAuth(), async ({ user, params, body }) => {
    const payload = body as { name?: unknown; size?: unknown }; if (typeof payload?.name !== 'string' || typeof payload.size !== 'number' || !Number.isSafeInteger(payload.size)) return error('A source filename and exact size are required.', 400);
    try { return engineResponse(await signedEngineRequest(user!.userId, 'POST', '/api/v1/projects/' + encodeURIComponent(params.projectId) + '/source-upload', payload)); }
    catch (err) { return error(err instanceof Error ? err.message : 'Source upload request failed.', 503); }
  }],
  'POST /api/engine/projects/:projectId/source-finalize': [requireAuth(), async ({ user, params, body }) => {
    try { return engineResponse(await signedEngineRequest(user!.userId, 'POST', '/api/v1/projects/' + encodeURIComponent(params.projectId) + '/source-finalize', body)); }
    catch (err) { return error(err instanceof Error ? err.message : 'Source finalization failed.', 503); }
  }],
  'POST /api/engine/projects/:projectId/builds': [requireAuth(), async ({ user, params }) => {
    try { return engineResponse(await signedEngineRequest(user!.userId, 'POST', '/api/v1/projects/' + encodeURIComponent(params.projectId) + '/builds', {})); }
    catch (err) { return error(err instanceof Error ? err.message : 'Could not queue the build.', 503); }
  }],
  'GET /api/engine/builds/:jobId': [requireAuth(), async ({ user, params }) => {
    try { return engineResponse(await signedEngineRequest(user!.userId, 'GET', '/api/v1/builds/' + encodeURIComponent(params.jobId))); }
    catch (err) { return error(err instanceof Error ? err.message : 'Could not read build status.', 503); }
  }],
  'POST /api/engine/builds/:jobId/cancel': [requireAuth(), async ({ user, params }) => {
    try { return engineResponse(await signedEngineRequest(user!.userId, 'POST', '/api/v1/builds/' + encodeURIComponent(params.jobId) + '/cancel', {})); }
    catch (err) { return error(err instanceof Error ? err.message : 'Could not cancel build.', 503); }
  }],
  'GET /api/engine/projects/:projectId/deployments': [requireAuth(), async ({ user, params }) => {
    try { return engineResponse(await signedEngineRequest(user!.userId, 'GET', '/api/v1/projects/' + encodeURIComponent(params.projectId) + '/deployments')); }
    catch (err) { return error(err instanceof Error ? err.message : 'Could not list deployments.', 503); }
  }],
  'POST /api/projects/:id/engine-link': [requireAuth(), async ({ user, params, body }) => {
    const payload = body as { engineProjectId?: unknown; slug?: unknown }; if (typeof payload?.engineProjectId !== 'string' || typeof payload.slug !== 'string') return error('Engine project identifier and slug are required.', 400);
    const table = tableFor(user!.userId); const [record] = await db.get<ProjectRecord>(table, [params.id]);
    if (!record || record.ownerId !== user!.userId) return error('Project draft not found.', 404);
    const [updated] = await db.tx.update(table, [{ id: params.id, record: { engineProjectId: payload.engineProjectId, slug: payload.slug } }], { patch: true });
    if (!updated) return error('Could not link the project to the Go engine.', 503);
    return json({ linked: true, projectId: params.id, engineProjectId: payload.engineProjectId, slug: payload.slug });
  }],
  'POST /api/projects/:id/engine-state': [requireAuth(), async ({ user, params, body }) => {
    const payload = body as { sourceUploaded?: unknown; deploymentStatus?: unknown; buildJobId?: unknown; deploymentId?: unknown };
    const table = tableFor(user!.userId); const [record] = await db.get<ProjectRecord>(table, [params.id]);
    if (!record || record.ownerId !== user!.userId) return error('Project draft not found.', 404);
    const patch: Record<string, unknown> = {};
    if (typeof payload?.sourceUploaded === 'boolean') { patch.sourceUploaded = payload.sourceUploaded; if (payload.sourceUploaded) patch.files = record.files.map(file => ({ ...file, uploaded: true })); }
    if (typeof payload?.deploymentStatus === 'string' && payload.deploymentStatus.length <= 40) patch.deploymentStatus = payload.deploymentStatus;
    if (typeof payload?.buildJobId === 'string' && payload.buildJobId.length <= 100) patch.buildJobId = payload.buildJobId;
    if (typeof payload?.deploymentId === 'string' && payload.deploymentId.length <= 100) patch.deploymentId = payload.deploymentId;
    if (!Object.keys(patch).length) return error('No valid engine state fields were supplied.', 400);
    const [updated] = await db.tx.update(table, [{ id: params.id, record: patch }], { patch: true });
    if (!updated) return error('Could not update project engine state.', 503); return json({ updated: true });
  }],
  'GET /api/health': [async () => json({
    status: 'ok',
    service: 'hosting-control-plane',
    privateObjectStorage: 'connected',
    projectPreflight: 'connected',
    buildEngine: 'not-connected'
  })],
  'GET /api/projects': [requireAuth(), async ({ user }) => {
    const page = await db.list<ProjectRecord>(tableFor(user!.userId), { limit: 50 });
    return json({ projects: page.items.map(item => publicProject(item.id, item)) });
  }],
  'POST /api/projects': [requireAuth(), async ({ user, body }) => {
    if (!body || typeof body !== 'object') return error('A project payload is required.', 400);
    const payload = body as { name?: unknown; files?: unknown };
    if (!validName(payload.name)) return error('Project name must contain 2–48 characters.', 400);
    if (!Array.isArray(payload.files) || payload.files.length < 1 || payload.files.length > 30) {
      return error('Choose between 1 and 30 supported files.', 400);
    }

    const files: ProjectFile[] = [];
    let totalBytes = 0;
    for (const raw of payload.files) {
      if (!raw || typeof raw !== 'object') return error('Invalid file metadata.', 400);
      const file = raw as Record<string, unknown>;
      if (
        typeof file.name !== 'string' ||
        !safeFileName(file.name) ||
        typeof file.size !== 'number' ||
        !Number.isSafeInteger(file.size) ||
        file.size < 0 ||
        file.size > 2 * 1024 * 1024 ||
        typeof file.kind !== 'string' ||
        file.kind.length > 80
      ) {
        return error('One or more files contain invalid metadata or exceed the 2 MB per-file limit.', 400);
      }
      if (!/\.(zip|html?|jsx|tsx?|css|vue|json)$/i.test(file.name)) {
        return error('Unsupported file type: ' + file.name, 400);
      }
      totalBytes += file.size;
      if (!Number.isSafeInteger(totalBytes) || totalBytes > 20 * 1024 * 1024) {
        return error('Project exceeds the 20 MB upload limit.', 413);
      }
      files.push({ name: file.name, size: file.size, kind: file.kind, uploaded: false });
    }

    const record: ProjectRecord = {
      ownerId: user!.userId,
      name: payload.name.trim(),
      framework: 'Pending preflight',
      status: 'draft',
      files,
      fileCount: files.length,
      totalBytes,
      createdAt: new Date().toISOString()
    };
    const [id] = await db.add(tableFor(user!.userId), [record as unknown as Record<string, unknown>]);
    if (!id) return error('The project draft could not be saved. Please retry.', 503);
    return json({ project: publicProject(id, record), sourceUploaded: false, deployed: false }, 201);
  }],
  'PUT /api/projects/:id/files/:index': [requireAuth(), async ({ user, params, body }) => {
    const index = Number(params.index);
    if (!Number.isSafeInteger(index) || index < 0 || index > 29) return error('Invalid file index.', 400);
    if (!body || typeof body !== 'object') return error('File content is required.', 400);
    const payload = body as { contentBase64?: unknown; contentType?: unknown };
    if (
      typeof payload.contentBase64 !== 'string' ||
      payload.contentBase64.length < 1 ||
      payload.contentBase64.length > 2_800_000 ||
      !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(payload.contentBase64)
    ) {
      return error('Invalid or oversized base64 file content.', 413);
    }

    const table = tableFor(user!.userId);
    const [record] = await db.get<ProjectRecord>(table, [params.id]);
    if (!record || record.ownerId !== user!.userId) return error('Project draft not found.', 404);
    const target = record.files[index];
    if (!target) return error('File entry not found.', 404);

    const actualBytes = Math.floor(payload.contentBase64.length * 3 / 4) -
      (payload.contentBase64.endsWith('==') ? 2 : payload.contentBase64.endsWith('=') ? 1 : 0);
    if (actualBytes !== target.size || actualBytes > 2 * 1024 * 1024) {
      return error('Uploaded bytes do not match the declared file size.', 400);
    }

    const contentType = contentTypeFor(target.name, payload.contentType);
    if (!contentType) return error('Unsupported source file type.', 400);
    const userKey = user!.userId.replace(/[^A-Za-z0-9]/g, '');
    const storagePath = 'source/' + userKey + '/' + params.id + '/' + index + '-' +
      target.name.replace(/[^A-Za-z0-9._-]/g, '_');
    const [stored] = await storage.write([{ path: storagePath, content: payload.contentBase64, contentType }]);
    if (!stored) return error('Private file storage failed. Retry this file.', 503);

    const nextFiles = record.files.map((file, fileIndex) =>
      fileIndex === index ? { ...file, uploaded: true, storagePath, contentType } : file
    );
    const [updated] = await db.tx.update(table, [{ id: params.id, record: { files: nextFiles } }], { patch: true });
    if (!updated) return error('File was stored, but project metadata could not be updated. Retry the same file safely.', 503);
    return json({ uploaded: true, index, fileName: target.name, bytes: actualBytes });
  }],
  'POST /api/projects/:id/analyze': [requireAuth(), async ({ user, params }) => {
    const table = tableFor(user!.userId);
    const [record] = await db.get<ProjectRecord>(table, [params.id]);
    if (!record || record.ownerId !== user!.userId) return error('Project draft not found.', 404);
    if (!record.files.every(file => file.uploaded)) {
      return error('Upload every project file before running preflight analysis.', 409);
    }
    const analysis = analyzeProject(record.files);
    const [updated] = await db.tx.update(table, [{
      id: params.id,
      record: { framework: analysis.framework, analysis }
    }], { patch: true });
    if (!updated) return error('Project analysis could not be saved. Please retry.', 503);
    return json({ project: publicProject(params.id, { ...record, framework: analysis.framework, analysis }) });
  }],
  'DELETE /api/projects/:id': [requireAuth(), async ({ user, params }) => {
    const table = tableFor(user!.userId);
    const [record] = await db.get<ProjectRecord>(table, [params.id]);
    if (!record || record.ownerId !== user!.userId) return error('Project draft not found.', 404);
    const paths = record.files
      .map(file => file.storagePath)
      .filter((path): path is string => typeof path === 'string');
    if (paths.length) {
      const results = await storage.delete(paths);
      if (results.some(ok => !ok)) {
        return error('Some stored source files could not be removed. Project record was retained.', 503);
      }
    }
    const [deleted] = await db.delete(table, [params.id]);
    if (!deleted) return error('Project draft could not be deleted.', 409);
    return json({ deleted: true, id: params.id, storedFilesDeleted: paths.length });
  }]
});
