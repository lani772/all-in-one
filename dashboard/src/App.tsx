import { useEffect, useMemo, useRef, useState } from 'react';
import { api, auth } from '@appdeploy/client';
import {
  Activity,
  ArrowRight,
  Bell,
  Box,
  Check,
  ChevronDown,
  CircleHelp,
  Cloud,
  Code2,
  FileCode2,
  FileUp,
  FolderGit2,
  Globe2,
  HardDrive,
  LayoutDashboard,
  LifeBuoy,
  LoaderCircle,
  LockKeyhole,
  Menu,
  MoreHorizontal,
  Plus,
  Rocket,
  Search,
  Settings2,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  UploadCloud,
  X,
  Zap,
} from 'lucide-react';

type View =
  | 'Overview'
  | 'Projects'
  | 'New deployment'
  | 'Deployments'
  | 'Domains'
  | 'Settings';
type SelectedFile = { file: File; kind: string };
type ProjectAnalysis = {
  framework: string;
  packageManager: string;
  deploymentTarget: string;
  entrypoint: string;
  confidence: 'high' | 'medium' | 'low';
  warnings: string[];
  checkedAt: string;
};
type Project = {
  id: string;
  name: string;
  framework: string;
  analysis?: ProjectAnalysis | null;
  status: 'draft';
  fileCount: number;
  totalBytes: number;
  createdAt: string;
  uploadedCount?: number;
  uploadedBytes?: number;
  engineProjectId?: string; slug?: string; sourceUploaded?: boolean; deploymentStatus?: string; buildJobId?: string; deploymentId?: string;
};
type EngineStatus = { configured: boolean; ready: boolean; message?: string; buildExecution?: string; publicBaseUrl?: string | null };

const acceptedExtensions = [
  '.zip',
  '.html',
  '.htm',
  '.jsx',
  '.tsx',
  '.js',
  '.ts',
  '.css',
  '.vue',
  '.json',
];

function detectKind(fileName: string) {
  const ext = fileName.toLowerCase().slice(fileName.lastIndexOf('.'));
  if (ext === '.zip') return 'Project archive';
  if (['.html', '.htm'].includes(ext)) return 'Static HTML';
  if (['.jsx', '.tsx', '.js', '.ts'].includes(ext))
    return 'JavaScript / React source';
  if (ext === '.vue') return 'Vue component';
  if (ext === '.css') return 'Stylesheet';
  if (ext === '.json') return 'Project configuration';
  return 'Unsupported file';
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

function crc32(bytes: Uint8Array) { let c = 0xffffffff; for (const b of bytes) { c ^= b; for (let i = 0; i < 8; i += 1) c = (c >>> 1) ^ (c & 1 ? 0xedb88320 : 0); } return (c ^ 0xffffffff) >>> 0; }
async function makeProjectArchive(selected: SelectedFile[], projectName: string): Promise<File> {
  if (selected.length === 1 && /\.zip$/i.test(selected[0].file.name)) return selected[0].file;
  if (selected.some(x => /\.zip$/i.test(x.file.name))) throw new Error('Upload a ZIP by itself, or select individual files to package into a ZIP.');
  const enc = new TextEncoder(), local: Uint8Array[] = [], central: Uint8Array[] = []; let offset = 0;
  for (const item of selected) { const name = enc.encode(item.file.name), data = new Uint8Array(await item.file.arrayBuffer()), crc = crc32(data);
    const lh = new Uint8Array(30 + name.length), lv = new DataView(lh.buffer); lv.setUint32(0,0x04034b50,true); lv.setUint16(4,20,true); lv.setUint16(6,0x0800,true); lv.setUint16(8,0,true); lv.setUint16(10,0,true); lv.setUint16(12,0x21,true); lv.setUint32(14,crc,true); lv.setUint32(18,data.length,true); lv.setUint32(22,data.length,true); lv.setUint16(26,name.length,true); lv.setUint16(28,0,true); lh.set(name,30); local.push(lh,data);
    const ch = new Uint8Array(46 + name.length), cv = new DataView(ch.buffer); cv.setUint32(0,0x02014b50,true); cv.setUint16(4,20,true); cv.setUint16(6,20,true); cv.setUint16(8,0x0800,true); cv.setUint16(10,0,true); cv.setUint16(12,0,true); cv.setUint16(14,0x21,true); cv.setUint32(16,crc,true); cv.setUint32(20,data.length,true); cv.setUint32(24,data.length,true); cv.setUint16(28,name.length,true); cv.setUint16(30,0,true); cv.setUint16(32,0,true); cv.setUint16(34,0,true); cv.setUint16(36,0,true); cv.setUint32(38,0,true); cv.setUint32(42,offset,true); ch.set(name,46); central.push(ch); offset += lh.length + data.length;
  }
  const csize = central.reduce((n,x)=>n+x.length,0), end = new Uint8Array(22), ev = new DataView(end.buffer); ev.setUint32(0,0x06054b50,true); ev.setUint16(4,0,true); ev.setUint16(6,0,true); ev.setUint16(8,selected.length,true); ev.setUint16(10,selected.length,true); ev.setUint32(12,csize,true); ev.setUint32(16,offset,true); ev.setUint16(20,0,true);
  const name = (projectName.trim() || 'project').replace(/[^A-Za-z0-9_-]/g,'-');
  const parts: BlobPart[] = [...local, ...central, end].map(part => { const copy = new Uint8Array(part.byteLength); copy.set(part); return copy.buffer; });
  return new File(parts,name+'.zip',{type:'application/zip'});
}

function App() {
  const [view, setView] = useState<View>('Overview');
  const [files, setFiles] = useState<SelectedFile[]>([]);
  const [projectName, setProjectName] = useState('');
  const [dragging, setDragging] = useState(false);
  const [mobileMenu, setMobileMenu] = useState(false);
  const [notice, setNotice] = useState('');
  const [user, setUser] = useState<{ userId: string; email?: string; name?: string } | null>(null);
  const [authReady, setAuthReady] = useState(false);
  const [projects, setProjects] = useState<Project[]>([]);
  const [engineStatus, setEngineStatus] = useState<EngineStatus>({ configured: false, ready: false, message: 'Engine credentials are not configured.' });
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const totalSize = useMemo(
    () => files.reduce((sum, item) => sum + item.file.size, 0),
    [files]
  );
  const unsupported = files.filter(item => item.kind === 'Unsupported file');

  useEffect(() => {
    let active = true;
    async function restoreSession() {
      try {
        const current = await auth.getUser();
        if (!active) return;
        setUser(current ? { userId: current.userId, email: current.email, name: current.name } : null);
        if (current) {
          const [response, engine] = await Promise.all([api.get('/api/projects'), api.get('/api/engine/status')]);
          if (active) { setProjects(response.data.projects ?? []); setEngineStatus(engine.data as EngineStatus); }
        }
      } catch (err) {
        if (active) setNotice(err instanceof Error ? err.message : 'Could not restore your workspace. Please refresh.');
      } finally {
        if (active) setAuthReady(true);
      }
    }
    restoreSession();
    return () => { active = false; };
  }, []);

  async function handleSignIn() {
    try {
      await auth.signInWithRedirect();
    } catch (err) {
      setNotice(err instanceof Error ? err.message : 'Sign-in could not start. Please try again.');
    }
  }

  function fileToBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        const result = String(reader.result || '');
        const comma = result.indexOf(',');
        resolve(comma >= 0 ? result.slice(comma + 1) : '');
      };
      reader.onerror = () => reject(new Error('Could not read ' + file.name));
      reader.readAsDataURL(file);
    });
  }

  async function saveProjectDraft() {
    if (!files.length) {
      setNotice('Choose a ZIP archive or supported source files first.');
      return;
    }
    if (unsupported.length) {
      setNotice('Remove unsupported files before saving this project.');
      return;
    }
    if (files.some(item => item.file.size > 2 * 1024 * 1024)) {
      setNotice('Each file must be 2 MB or smaller in this upload release. Split larger projects into smaller files or ZIP parts.');
      return;
    }
    if (totalSize > 20 * 1024 * 1024) {
      setNotice('The total project upload limit is 20 MB for this release.');
      return;
    }
    const name = projectName.trim() || files[0].file.name.replace(/\.zip$/i, '').replace(/\.[^.]+$/, '');
    if (!name) {
      setNotice('Enter a project name.');
      return;
    }
    setSaving(true);
    setNotice('');
    try {
      const response = await api.post('/api/projects', {
        name,
        files: files.map(item => ({ name: item.file.name, size: item.file.size, kind: item.kind })),
      });
      const project = response.data.project as Project;
      if (engineStatus.configured) {
        if (!engineStatus.ready) throw new Error('The Go hosting engine is configured but not ready. Check PostgreSQL, Redis and object-storage readiness before uploading.');
        setNotice('Preparing a secure source archive…'); const archive = await makeProjectArchive(files, name);
        if (archive.size < 1 || archive.size > 20 * 1024 * 1024) throw new Error('The generated ZIP must be between 1 byte and 20 MB.');
        const remoteProject = (await api.post('/api/engine/projects', { name })).data;
        await api.post('/api/projects/' + encodeURIComponent(project.id) + '/engine-link', { engineProjectId: remoteProject.projectId, slug: remoteProject.slug });
        setNotice('Requesting a private source-upload URL…');
        const uploadInfo = (await api.post('/api/engine/projects/' + encodeURIComponent(remoteProject.projectId) + '/source-upload', { name: archive.name, size: archive.size })).data;
        const uploadForm = new FormData();
        for (const [field, value] of Object.entries(uploadInfo.fields as Record<string, string>)) uploadForm.append(field, value);
        uploadForm.append('file', archive, archive.name);
        const putResponse = await fetch(uploadInfo.uploadUrl, { method: 'POST', body: uploadForm });
        if (!putResponse.ok) throw new Error('Private object storage rejected the ZIP upload (HTTP ' + putResponse.status + '). Check storage CORS and the presigned URL configuration.');
        await api.post('/api/engine/projects/' + encodeURIComponent(remoteProject.projectId) + '/source-finalize', { objectKey: uploadInfo.objectKey });
        await api.post('/api/projects/' + encodeURIComponent(project.id) + '/engine-state', { sourceUploaded: true, deploymentStatus: 'uploaded' });
        setNotice('Source archive verified. Creating the build job…');
        const build = (await api.post('/api/engine/projects/' + encodeURIComponent(remoteProject.projectId) + '/builds', {})).data;
        await api.post('/api/projects/' + encodeURIComponent(project.id) + '/engine-state', { sourceUploaded: true, buildJobId: build.jobId, deploymentStatus: build.status });
        const refreshed = await api.get('/api/projects'); setProjects(refreshed.data.projects ?? []); setFiles([]); setProjectName(''); setView('Projects');
        setNotice(build.execution === 'inspection-only' ? 'Source uploaded and verified. The job is queued for inspection only; isolated build execution is disabled on the Go engine.' : 'Source uploaded and verified. Build job queued successfully.'); return;
      }
      setNotice('Project created. Uploading source files securely…');
      for (let index = 0; index < files.length; index += 1) {
        const selected = files[index];
        const contentBase64 = await fileToBase64(selected.file);
        await api.put('/api/projects/' + encodeURIComponent(project.id) + '/files/' + index, {
          contentBase64,
          contentType: selected.file.type || 'application/octet-stream',
        });
        setNotice('Uploaded ' + (index + 1) + ' of ' + files.length + ' files…');
      }
      setNotice('All files uploaded. Running project preflight…');
      await api.post('/api/projects/' + encodeURIComponent(project.id) + '/analyze', {});
      const refreshed = await api.get('/api/projects');
      setProjects(refreshed.data.projects ?? [project]);
      setFiles([]);
      setProjectName('');
      setView('Projects');
      setNotice('Source files uploaded to private project storage. Build and publishing are not connected yet.');
    } catch (err) {
      setNotice(err instanceof Error ? err.message : 'Could not save the project draft.');
    } finally {
      setSaving(false);
    }
  }

  async function refreshBuild(project: Project) {
    if (!project.buildJobId) return;
    try { const result = await api.get('/api/engine/builds/' + encodeURIComponent(project.buildJobId)); await api.post('/api/projects/' + encodeURIComponent(project.id) + '/engine-state', { deploymentStatus: result.data.status, deploymentId: result.data.deploymentId || undefined }); const refreshed = await api.get('/api/projects'); setProjects(refreshed.data.projects ?? []); setNotice('Build status refreshed: ' + result.data.status + '.'); }
    catch (err) { setNotice(err instanceof Error ? err.message : 'Could not refresh build status.'); }
  }
  async function deleteProject(projectId: string) {
    try {
      await api.delete('/api/projects/' + encodeURIComponent(projectId));
      setProjects(current => current.filter(project => project.id !== projectId));
      setNotice('Project draft deleted.');
    } catch (err) {
      setNotice(err instanceof Error ? err.message : 'Could not delete this project.');
    }
  }

  function addFiles(list: FileList | File[]) {
    const next = Array.from(list).map(file => ({
      file,
      kind: detectKind(file.name),
    }));
    setFiles(current => [...current, ...next].slice(0, 30));
    if (next.some(item => item.kind === 'Unsupported file')) {
      setNotice('Some files are not supported. Remove them before continuing.');
    } else {
      setNotice('');
    }
  }

  function chooseFiles() {
    inputRef.current?.click();
  }

  function nav(viewName: View) {
    setView(viewName);
    setMobileMenu(false);
    setNotice('');
  }

  const navItems: { label: View; icon: typeof LayoutDashboard }[] = [
    { label: 'Overview', icon: LayoutDashboard },
    { label: 'Projects', icon: Box },
    { label: 'New deployment', icon: UploadCloud },
    { label: 'Deployments', icon: Rocket },
    { label: 'Domains', icon: Globe2 },
  ];

  function renderWorkspace() {
    if (view === 'New deployment') {
      return (
        <div className="workspace">
          <div className="page-heading">
            <div>
              <div className="eyebrow">PUBLISH SOMETHING NEW</div>
              <h1>New deployment</h1>
              <p>
                Bring your frontend code. We’ll identify the framework and
                prepare the build.
              </p>
            </div>
            <span className="step-pill">
              <span className="step-dot" /> Step 1 of 3 · Upload
            </span>
          </div>
          <div className="deploy-layout">
            <section className="panel upload-panel">
              <div className="panel-title">
                <div>
                  <h2>Upload your project</h2>
                  <p>Choose a ZIP archive or add individual source files.</p>
                </div>
                <span className="icon-tile">
                  <FileUp size={19} />
                </span>
              </div>
              <div
                className={'dropzone ' + (dragging ? 'is-dragging' : '')}
                onDragOver={event => {
                  event.preventDefault();
                  setDragging(true);
                }}
                onDragLeave={() => setDragging(false)}
                onDrop={event => {
                  event.preventDefault();
                  setDragging(false);
                  addFiles(event.dataTransfer.files);
                }}
              >
                <div className="upload-illustration">
                  <UploadCloud size={27} />
                </div>
                <h3>Drag and drop your files here</h3>
                <p>Upload a complete project ZIP or select individual files.</p>
                <button className="button button-primary" onClick={chooseFiles}>
                  <Plus size={16} /> Choose files
                </button>
                <input
                  ref={inputRef}
                  type="file"
                  multiple
                  accept={acceptedExtensions.join(',')}
                  hidden
                  onChange={event => {
                    if (event.target.files) addFiles(event.target.files);
                    event.target.value = '';
                  }}
                />
                <div className="file-limits">
                  <span>
                    <Check size={13} /> ZIP projects
                  </span>
                  <span>
                    <Check size={13} /> HTML / CSS / JS
                  </span>
                  <span>
                    <Check size={13} /> JSX / TSX / Vue
                  </span>
                </div>
              </div>
              {files.length > 0 && (
                <div className="selected-files">
                  <div className="selected-head">
                    <div>
                      <h3>
                        Selected files{' '}
                        <span className="count">{files.length}</span>
                      </h3>
                      <p>{formatBytes(totalSize)} total · Maximum 2 MB per file / 20 MB per project</p>
                    </div>
                    <button
                      className="text-button"
                      onClick={() => {
                        setFiles([]);
                        setNotice('');
                      }}
                    >
                      Clear all
                    </button>
                  </div>
                  {files.map((item, index) => (
                    <div className="file-row" key={index}>
                      <div className="file-icon">
                        <FileCode2 size={17} />
                      </div>
                      <div className="file-info">
                        <strong>{item.file.name}</strong>
                        <span>
                          {item.kind} · {formatBytes(item.file.size)}
                        </span>
                      </div>
                      {item.kind === 'Unsupported file' ? (
                        <span className="status status-error">Unsupported</span>
                      ) : (
                        <span className="status status-neutral">Ready</span>
                      )}
                      <button
                        aria-label={'Remove ' + item.file.name}
                        className="icon-button"
                        onClick={() =>
                          setFiles(current =>
                            current.filter((_, i) => i !== index)
                          )
                        }
                      >
                        <X size={15} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              <div className="field-group">
                <label htmlFor="project-name">
                  Project name <span>Optional for now</span>
                </label>
                <input
                  id="project-name"
                  value={projectName}
                  onChange={event => setProjectName(event.target.value)}
                  placeholder="e.g. my-portfolio"
                  maxLength={48}
                />
                <small>
                  Will become part of your public project URL when hosting is
                  connected.
                </small>
              </div>
              {notice && <div className="notice notice-error">{notice}</div>}
              <div className="backend-notice">
                <LockKeyhole size={17} />
                <div>
                  <strong>Save a project draft first</strong>
                  <p>
                    When the Go engine is connected, individual files are packaged into a ZIP and uploaded to private object storage through a short-lived presigned URL. Builds run only on isolated workers; if sandbox execution is disabled, the engine performs inspection without publishing.
                  </p>
                </div>
              </div>
              <div className="form-footer">
                <span>
                  Saving creates a private draft only. It does not publish a live URL.
                </span>
                <button className="button button-primary" onClick={saveProjectDraft} disabled={saving || !files.length || unsupported.length > 0}>
                  {saving ? <LoaderCircle size={16} className="spin" /> : <Check size={16} />}
                  {saving ? 'Saving draft…' : 'Save project draft'}
                </button>
              </div>
            </section>
            <aside className="side-stack">
              <section className="panel side-panel">
                <div className="side-heading">
                  <Sparkles size={17} />
                  <h3>Automatic detection</h3>
                </div>
                <p>
                  After upload, the preflight checks filenames and common project entrypoints. ZIP contents remain uninspected until secure extraction is implemented.
                </p>
                <div className="framework-list">
                  {['React', 'Next.js', 'Vue', 'Vite', 'HTML / CSS / JS'].map(
                    name => (
                      <div key={name}>
                        <span className="framework-mark">
                          {name === 'Next.js'
                            ? 'N'
                            : name === 'Vue'
                              ? 'V'
                              : name === 'React'
                                ? 'R'
                                : name === 'Vite'
                                  ? '⚡'
                                  : '</>'}
                        </span>
                        {name}
                        <span className="framework-supported">Planned</span>
                      </div>
                    )
                  )}
                </div>
              </section>
              <section className="panel side-panel">
                <div className="side-heading">
                  <ShieldCheck size={17} />
                  <h3>Build isolation</h3>
                </div>
                <p>
                  Uploaded code must be built in isolated workers, never inside
                  the dashboard server.
                </p>
                <div className="security-note">
                  <LockKeyhole size={15} /> ZIP path validation · resource
                  limits · build timeouts
                </div>
              </section>
            </aside>
          </div>
        </div>
      );
    }

    const titles: Record<View, string> = {
      Overview: 'Your deployment workspace',
      Projects: 'Your projects',
      'New deployment': 'New deployment',
      Deployments: 'Deployment history',
      Domains: 'Domains',
      Settings: 'Workspace settings',
    };
    const descriptions: Record<View, string> = {
      Overview: 'A clear view of your projects, builds and hosting activity.',
      Projects: 'Manage every frontend project in one place.',
      'New deployment': '',
      Deployments: 'Track build activity and inspect deployment results.',
      Domains: 'Manage project URLs and custom domain connections.',
      Settings: 'Configure your hosting workspace.',
    };
    return (
      <div className="workspace">
        <div className="page-heading">
          <div>
            <div className="eyebrow">WORKSPACE / {view.toUpperCase()}</div>
            <h1>{titles[view]}</h1>
            <p>{descriptions[view]}</p>
          </div>
          <button
            className="button button-primary"
            onClick={() => nav('New deployment')}
          >
            <Plus size={16} /> New deployment
          </button>
        </div>
        {view === 'Overview' && (
          <div className="metric-grid">
            {[
              {
                label: 'Total projects',
                value: '0',
                icon: Box,
                hint: 'No projects yet',
              },
              {
                label: 'Live deployments',
                value: '0',
                icon: Globe2,
                hint: 'Nothing published',
              },
              {
                label: 'Build minutes',
                value: '—',
                icon: Activity,
                hint: 'Engine not connected',
              },
              {
                label: 'Storage used',
                value: '0 B',
                icon: HardDrive,
                hint: 'No files stored',
              },
            ].map(metric => (
              <div className="metric-card" key={metric.label}>
                <div className="metric-top">
                  <span>{metric.label}</span>
                  <metric.icon size={17} />
                </div>
                <strong>{metric.value}</strong>
                <small>{metric.hint}</small>
              </div>
            ))}
          </div>
        )}
        {view === 'Projects' && projects.length > 0 && (
          <section className="project-grid" aria-label="Saved project drafts">
            {projects.map(project => (
              <article className="panel project-card" key={project.id}>
                <div className="project-card-top">
                  <div className="icon-tile"><FolderGit2 size={19} /></div>
                  <span className={"status " + (project.deploymentStatus === "failed" ? "status-error" : "status-neutral")}>{project.deploymentStatus || "Draft"}</span>
                </div>
                <h2>{project.name}</h2>
                <p>{project.framework} · {project.fileCount} file{project.fileCount === 1 ? '' : 's'}</p>
                <div className="project-meta"><span>{formatBytes(project.uploadedBytes ?? 0)} uploaded</span><span>{project.uploadedCount ?? 0}/{project.fileCount} files</span></div>
                {project.slug && <div className="project-analysis"><strong>Hosting slug: {project.slug}</strong><span>Connected to Go deployment engine</span></div>}
                {project.buildJobId && <button className="text-button project-delete" onClick={() => refreshBuild(project)}>Refresh build status <Activity size={14} /></button>}
                {project.deploymentStatus === "succeeded" && project.slug && engineStatus.publicBaseUrl && <a className="text-button project-delete" href={engineStatus.publicBaseUrl.replace(/\/$/, "") + "/" + project.slug} target="_blank" rel="noreferrer">Open live deployment <ArrowRight size={14} /></a>}
                {project.analysis && <div className="project-analysis"><strong>{project.analysis.deploymentTarget}</strong><span>Package manager: {project.analysis.packageManager} · Entry: {project.analysis.entrypoint}</span>{project.analysis.warnings.length > 0 && <small>{project.analysis.warnings[0]}</small>}</div>}
                <button className="text-button project-delete" onClick={() => deleteProject(project.id)}>Delete draft <X size={14} /></button>
              </article>
            ))}
          </section>
        )}
        {!(view === 'Projects' && projects.length > 0) && (
        <section className="panel empty-panel">
          <div className="empty-art">
            <div className="empty-art-back">
              <Code2 size={34} />
            </div>
            <div className="empty-art-front">
              <Cloud size={31} />
            </div>
            <span className="sparkle sparkle-one">✦</span>
            <span className="sparkle sparkle-two">✧</span>
          </div>
          <h2>
            {view === 'Overview'
              ? 'Your next project starts here'
              : view === 'Projects'
                ? 'No projects yet'
                : view === 'Deployments'
                  ? 'No deployments yet'
                  : view === 'Domains'
                    ? 'No domains connected'
                    : 'Settings are coming online'}
          </h2>
          <p>
            {view === 'Overview'
              ? 'Upload a frontend project to begin building your hosting workspace.'
              : view === 'Projects'
                ? 'Upload a ZIP or individual files to create your first hosted project.'
                : view === 'Deployments'
                  ? 'Build and publish a project to see its deployment history here.'
                  : view === 'Domains'
                    ? 'Project subpaths will be available first. Custom domains can be connected later.'
                    : 'Workspace configuration will appear here as the hosting services are connected.'}
          </p>
          <button
            className="button button-primary"
            onClick={() => nav('New deployment')}
          >
            <UploadCloud size={16} /> Upload a project <ArrowRight size={15} />
          </button>
          <div className="empty-footnote">
            <span className="status-dot" /> Hosting engine setup in progress
          </div>
        </section>
        )}
        <div className="info-strip">
          <div className="info-icon">
            <TerminalSquare size={18} />
          </div>
          <div>
            <strong>Built for modern frontend projects</strong>
            <p>
              React, Next.js, Vue, Vite and static HTML projects — one workflow.
            </p>
          </div>
          <button className="text-button" onClick={() => nav('New deployment')}>
            Start now <ArrowRight size={14} />
          </button>
        </div>
      </div>
    );
  }

  if (!authReady) {
    return <div className="auth-screen"><div className="auth-card"><div className="brand-symbol"><span /></div><h1>Preparing your workspace</h1><p>Restoring your secure session…</p><LoaderCircle className="spin" size={22} /></div></div>;
  }

  if (!user) {
    return <div className="auth-screen"><div className="auth-card"><div className="brand-symbol"><span /></div><div className="eyebrow">ALL-IN-ONE HOSTING</div><h1>Your frontend deployment workspace</h1><p>Sign in to create private project drafts and manage your hosting workspace.</p><button className="button button-primary auth-button" onClick={handleSignIn}>Continue securely <ArrowRight size={16} /></button>{notice && <div className="notice notice-error">{notice}</div>}<small>Your project records are scoped to your signed-in account.</small></div></div>;
  }

  return (
    <div className="app-shell">
      <aside className={'sidebar ' + (mobileMenu ? 'sidebar-open' : '')}>
        <div className="brand">
          <div className="brand-symbol">
            <span />
          </div>
          <span>
            allinone<span className="brand-muted">.host</span>
          </span>
          <button
            className="mobile-close icon-button"
            onClick={() => setMobileMenu(false)}
          >
            <X size={18} />
          </button>
        </div>
        <div className="workspace-switch">
          <div className="workspace-avatar">A</div>
          <div>
                          <strong>{user.name || 'Personal workspace'}</strong>
              <span>{user.email || 'Private workspace'}</span>
          </div>
          <ChevronDown size={15} />
        </div>
        <div className="nav-label">WORKSPACE</div>
        <nav className="main-nav">
          {navItems.map(item => (
            <button
              key={item.label}
              className={'nav-item ' + (view === item.label ? 'active' : '')}
              onClick={() => nav(item.label)}
            >
              <item.icon size={17} strokeWidth={1.8} />
              <span>{item.label}</span>
              {item.label === 'New deployment' && (
                <span className="nav-plus">+</span>
              )}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="usage-card">
            <div className="usage-title">
              <Zap size={15} />
              <strong>Build capacity</strong>
            </div>
            <div className="usage-line">
              <span>Build engine</span>
              <span className={engineStatus.ready ? "framework-supported" : "coming-soon"}>{engineStatus.ready ? "Connected" : engineStatus.configured ? "Unhealthy" : "Not configured"}</span>
            </div>
            <div className="usage-track">
              <span />
            </div>
            <p>{engineStatus.message || "Add ENGINE_API_URL and AUTH_HMAC_SECRET in secure app settings to enable deployments."}</p>
          </div>
          <button
            className={'nav-item ' + (view === 'Settings' ? 'active' : '')}
            onClick={() => nav('Settings')}
          >
            <Settings2 size={17} />
            <span>Settings</span>
          </button>
          <button
            className="nav-item"
            onClick={() => setNotice('Help center will be available soon.')}
          >
            <LifeBuoy size={17} />
            <span>Help & support</span>
          </button>
          <div className="profile-row">
            <div className="profile-avatar">U</div>
            <div>
              <strong>{user.name || 'Your workspace'}</strong>
              <span>{user.email || 'Signed in'}</span>
            </div>
            <MoreHorizontal size={18} />
          </div>
        </div>
      </aside>
      {mobileMenu && (
        <button
          className="mobile-scrim"
          aria-label="Close navigation"
          onClick={() => setMobileMenu(false)}
        />
      )}
      <main className="main-area">
        <header className="topbar">
          <div className="topbar-left">
            <button
              className="mobile-menu icon-button"
              onClick={() => setMobileMenu(true)}
            >
              <Menu size={19} />
            </button>
            <div className="breadcrumb">
              Workspace <span>/</span> <strong>{view}</strong>
            </div>
          </div>
          <div className="topbar-actions">
            <div className="environment-badge">
              <span /> Development
            </div>
            <button className="icon-button" aria-label="Search">
              <Search size={18} />
            </button>
            <button
              className="icon-button notification-button"
              aria-label="Notifications"
            >
              <Bell size={18} />
              <i />
            </button>
            <div className="top-avatar">U</div>
          </div>
        </header>
        {renderWorkspace()}
        {notice && view !== 'New deployment' && (
          <div className="toast">
            <CircleHelp size={16} />
            {notice}
            <button onClick={() => setNotice('')}>
              <X size={14} />
            </button>
          </div>
        )}
      </main>
    </div>
  );
}

export default App;
