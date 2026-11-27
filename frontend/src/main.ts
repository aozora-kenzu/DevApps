import './style.css';
import {
    AddCustomDirToPath,
    AddToPath,
    ApplyAppUpdate,
    CancelActiveTask,
    CancelUnifiedSearch,
    CheckAppUpdate,
    CleanInvalidUserPath,
    ConfirmCloseAction,
    DeleteInstalledVersion,
    DownloadAndInstall,
    EnrichWingetSearch,
    GetDashboardState,
    GetUniversalPackageDetails,
    HideToTray,
    IgnoreInstalledApp,
    InstallCLITool,
    InstallSelectedEssentials,
    InstallWingetByID,
    InstallWingetCLI,
    MinimizeWindow,
    MoveUserPathEntry,
    OpenAppPathFolder,
    OpenEssentialsConf,
    OpenExternalURL,
    OpenIgnoredConf,
    OpenPackagesConf,
    OpenUserPathDir,
    PauseInstalledAppUpdates,
    PickAndAddUserPathDir,
    PickOutputDirectory,
    PinWingetApp,
    QuitApp,
    RefreshUserPath,
    RefreshWingetStatus,
    ReloadConfig,
    RemoveFromPath,
    RemoveUserPathEntry,
    RestartAsAdmin,
    RunEssentialTool,
    RunPackageAction,
    SaveAppSettings,
    SetLanguage,
    SkipInstalledAppVersion,
    StartUnifiedSearch,
    SyncDefaultPackages,
    SyncPowerState,
    UnignoreInstalledApp,
    UpdateAllInstalled,
    UpdateLTSLinks,
    UpdateWingetByID,
} from '../wailsjs/go/main/App';
import { config, essentials, main, pkgmgr, tools } from '../wailsjs/go/models';
import { EventsOn, Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime/runtime';
import {
    getT,
    Lang,
    localizeEssentialDesc,
    localizePkgDesc,
    localizeToolDesc,
    localizeToolRequires,
} from './i18n';
import { getCuratedMeta, renderAppIcon } from './icons';

type PrimaryTab = 'runtimes' | 'essentials';
type RuntimeTabKey = 'all' | 'core' | 'stack' | 'pkgmgr' | 'tools' | 'path';
type EssentialTabKey =
    | 'all'
    | 'browsers'
    | 'utilities'
    | 'editors'
    | 'office'
    | 'chat'
    | 'downloaders'
    | 'installed'
    | 'search';
type InstalledFilterKey = 'updates' | 'all' | 'ignored';
type ManagerKey =
    | 'winget'
    | 'scoop'
    | 'choco'
    | 'npm'
    | 'pnpm'
    | 'bun'
    | 'uv'
    | 'pip'
    | 'cargo'
    | 'dotnet'
    | 'psgallery';
type InstalledMgrFilterKey = 'all' | ManagerKey;
type SearchFilterKey = 'all' | 'winget' | 'msstore' | 'scoop' | 'choco' | 'js' | 'py' | 'cargo' | 'dotnet' | 'psgallery';

interface UnifiedSearchChunkEvent {
    token: number;
    query: string;
    sourceGroup: string;
    jsMgrs?: string[];
    pyMgrs?: string[];
    wingetRows?: essentials.WingetTableRow[];
    pkgRows?: pkgmgr.PackageEntry[];
    error?: string;
    completed: number;
    total: number;
    pending?: string[];
    done: boolean;
}

interface ProgressState {
    stage: string;
    percent: number;
    downloaded: number;
    total: number;
    speedBps: number;
    detail: string;
}

interface DetailTarget {
    manager?: string;
    appId?: string;
    wingetId: string;
    name: string;
    source: string;
    version?: string;
    fallbackDesc?: string;
    iconUrl?: string;
    isEssentialItem?: boolean;
}

let state: main.DashboardState | null = null;
let primaryTab: PrimaryTab = 'runtimes';
let runtimeTab: RuntimeTabKey = 'all';
let essentialTab: EssentialTabKey = 'all';
let installedFilter: InstalledFilterKey = 'updates';
let installedMgrFilter: InstalledMgrFilterKey = 'all';

let runtimeSearchQuery = '';
let essentialSearchQuery = '';
let wingetCatalogQuery = '';
let searchResultFilter: SearchFilterKey = 'all';
let wingetSearchResults: essentials.WingetTableRow[] = [];
let extraSearchByGroup: Record<string, pkgmgr.PackageEntry[]> = {};
let unifiedJSMgrs: ManagerKey[] = [];
let unifiedPyMgrs: ManagerKey[] = [];
let selectedSearchJSMgrByPkg: Record<string, ManagerKey> = {};
let unifiedSearchPending: string[] = [];
let unifiedSearchTotal = 0;
let unifiedSearchDone = false;
let wingetSearching = false;
let wingetEnriching = false;
let wingetSearchExecuted = false;
let wingetSearchToken = 0;
let wingetDetailsByKey: Record<string, essentials.PackageDetails> = {};
let customEssHydrated: Record<string, boolean> = {};

let detailTarget: DetailTarget | null = null;
let detailLoading = false;
let detailError = '';

let selectedVersionByApp: Record<string, string> = {};
let selectedEssentialVerByApp: Record<string, string> = {};
let checkedEssentials: Record<string, boolean> = {};

let logs: main.TerminalLogEvent[] = [];
let initialLogsLoaded = false;
let terminalCollapsed = false;
let busy = false;
let showSettingsModal = false;
let settingsDraft: config.AppSettings | null = null;
let showCloseConfirmModal = false;
let closeConfirmRemember = false;
let showAboutModal = false;
let checkingAppUpdate = false;
let windowMaximized = false;

let scrollByViewKey: Record<string, number> = {};
let sidebarSummaryScrollTop = 0;

let progress: ProgressState = {
    stage: 'idle',
    percent: 0,
    downloaded: 0,
    total: 0,
    speedBps: 0,
    detail: '',
};

const appRoot = document.querySelector<HTMLDivElement>('#app')!;

function currentViewKey(): string {
    if (primaryTab === 'runtimes') {
        return `rt:${runtimeTab}`;
    }
    return essentialTab === 'installed' ? `es:installed:${installedFilter}` : `es:${essentialTab}`;
}

function currentLang(): Lang {
    return state?.language === 'id' ? 'id' : 'en';
}

function escapeHtml(text: string): string {
    return text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function formatBytes(bytes: number): string {
    if (!bytes || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let val = bytes;
    let idx = 0;
    while (val >= 1024 && idx < units.length - 1) {
        val /= 1024;
        idx++;
    }
    return `${val.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`;
}

function appendLog(entry: main.TerminalLogEvent): void {
    logs.push(entry);
    if (logs.length > 500) {
        logs = logs.slice(logs.length - 500);
    }
    renderTerminalLinesOnly();
}

function pickDefaultSelectedVersion(pkg: main.PackageView): string {
    const remembered = selectedVersionByApp[pkg.id];
    if (remembered && pkg.versions.some((v) => v.version === remembered)) {
        return remembered;
    }
    if (pkg.activeVer && pkg.versions.some((v) => v.version === pkg.activeVer)) {
        return pkg.activeVer;
    }
    const installedVer = pkg.versions.find((v) => v.installed);
    if (installedVer) {
        return installedVer.version;
    }
    return pkg.versions.length > 0 ? pkg.versions[0].version : '';
}

function pickDefaultEssentialVersion(item: main.EssentialView): string {
    const versions = item.versions || [];
    if (versions.length === 0) return 'latest';
    const remembered = selectedEssentialVerByApp[item.id];
    if (remembered && versions.includes(remembered)) {
        return remembered;
    }
    return versions[0];
}

function matchesSearch(text: string, query: string): boolean {
    if (!query.trim()) return true;
    return text.toLowerCase().includes(query.trim().toLowerCase());
}

function detailKey(wingetId: string, source?: string, manager?: string): string {
    const mgr = (manager || 'winget').trim().toLowerCase();
    const idLower = wingetId.trim().toLowerCase();
    const srcLower = (source || mgr).trim().toLowerCase();
    return mgr === 'winget' ? `${idLower}|${srcLower}` : `${mgr}|${idLower}|${srcLower}`;
}

function getCachedDetails(wingetId: string, source?: string, manager?: string): essentials.PackageDetails | undefined {
    const mgr = (manager || 'winget').trim().toLowerCase();
    if (mgr !== 'winget') {
        return wingetDetailsByKey[detailKey(wingetId, source, mgr)];
    }
    return (
        wingetDetailsByKey[detailKey(wingetId, source, 'winget')] ||
        wingetDetailsByKey[detailKey(wingetId, 'winget', 'winget')] ||
        wingetDetailsByKey[detailKey(wingetId, 'msstore', 'winget')]
    );
}

function formatDownloadCount(n: number): string {
    if (!n || n <= 0) return '';
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return `${n}`;
}

function shortDomain(rawUrl?: string): string {
    if (!rawUrl) return '';
    try {
        const u = new URL(rawUrl.includes('://') ? rawUrl : `https://${rawUrl}`);
        return u.hostname.replace(/^www\./i, '');
    } catch {
        return '';
    }
}

function renderMetaStrip(parts: { publisher?: string; domain?: string; license?: string; installerType?: string; match?: string }): string {
    const items: string[] = [];
    if (parts.publisher) {
        items.push(`<span class="meta-pill" title="${escapeHtml(parts.publisher)}">${escapeHtml(parts.publisher)}</span>`);
    }
    if (parts.domain) {
        items.push(`<span class="meta-pill meta-pill-domain" title="${escapeHtml(parts.domain)}">${escapeHtml(parts.domain)}</span>`);
    }
    if (parts.license) {
        items.push(`<span class="meta-pill" title="${escapeHtml(parts.license)}">${escapeHtml(parts.license)}</span>`);
    }
    if (parts.installerType) {
        items.push(`<span class="meta-pill">${escapeHtml(parts.installerType)}</span>`);
    }
    if (parts.match) {
        items.push(`<span class="meta-pill">${escapeHtml(parts.match)}</span>`);
    }
    return items.length > 0 ? `<div class="meta-strip">${items.join('')}</div>` : '';
}

function renderProgressInline(): string {
    if (!busy && progress.stage === 'idle') {
        return '';
    }
    const t = getT(currentLang());
    let label = progress.detail || t.progressProcessing;
    if (progress.stage === 'download') {
        const prefix = progress.detail ? `${progress.detail} · ` : '';
        const pctPart = progress.percent >= 0 ? `${progress.percent.toFixed(1)}%` : '';
        const totalText = progress.total > 0 ? ` / ${formatBytes(progress.total)}` : '';
        const bytesPart = progress.downloaded > 0 ? `${formatBytes(progress.downloaded)}${totalText}` : '';
        const speedPart = progress.speedBps > 0 ? `${formatBytes(progress.speedBps)}/s` : '';
        const parts = [pctPart, bytesPart, speedPart].filter(Boolean).join(' · ');
        label = `${prefix}DL${parts ? ` ${parts}` : ''}`;
    } else if (progress.stage === 'extract') {
        label = progress.detail || `Extract ${progress.percent.toFixed(0)}%`;
    } else if (progress.stage === 'install') {
        label = progress.detail || t.progressInstalling;
    }

    const fillClass = progress.percent < 0 ? 'progress-fill indeterminate' : 'progress-fill';
    const fillWidth = progress.percent < 0 ? '' : `style="width: ${Math.max(0, Math.min(100, progress.percent))}%"`;

    return `
        <div class="progress-inline">
            <div class="progress-track"><div class="${fillClass}" ${fillWidth}></div></div>
            <span>${escapeHtml(label)}</span>
        </div>
    `;
}

function renderVersionComparison(pkg: main.PackageView): string {
    const t = getT(currentLang());
    const activeText = pkg.winActivePath
        ? `<span class="vc-val">v${escapeHtml(pkg.winActiveVer || '?')}</span> <span class="vc-scope" title="${escapeHtml(pkg.winActivePath)}">(${escapeHtml(pkg.winActiveScope)})</span>`
        : `<span class="vc-scope">${escapeHtml(t.notInPath)}</span>`;

    const hasDistinctBranch =
        Boolean(pkg.branchLatestVer) &&
        Boolean(pkg.latestVer) &&
        pkg.branchLatestVer !== pkg.latestVer &&
        Boolean(pkg.branchPrefix);

    const versionCompareItems = hasDistinctBranch
        ? `
            <span class="vc-sep">·</span>
            <span class="vc-item">
                <span class="vc-label">${escapeHtml(t.latestBranchLabel(pkg.branchPrefix))}</span>
                <span class="vc-val">v${escapeHtml(pkg.branchLatestVer)}</span>
            </span>
            <span class="vc-sep">·</span>
            <span class="vc-item">
                <span class="vc-label">${escapeHtml(t.latestMainLabel)}</span>
                <span class="vc-val">v${escapeHtml(pkg.latestVer)}</span>
            </span>
        `
        : `
            <span class="vc-sep">·</span>
            <span class="vc-item">
                <span class="vc-label">${escapeHtml(t.latestLabel)}</span>
                ${pkg.latestVer ? `<span class="vc-val">v${escapeHtml(pkg.latestVer)}</span>` : `<span class="vc-scope">—</span>`}
            </span>
        `;

    let cmpBadge = `<span class="badge badge-idle">${escapeHtml(t.notInPath)}</span>`;
    if (pkg.winActivePath) {
        if (hasDistinctBranch) {
            if (pkg.branchStatus === 'outdated') {
                cmpBadge = `<span class="badge badge-installed">${escapeHtml(t.statusUpdateBranch(pkg.branchPrefix, pkg.branchLatestVer))}</span>`;
            } else if (pkg.branchStatus === 'uptodate') {
                cmpBadge = `<span class="badge badge-active">${escapeHtml(t.statusUpToDateBranch(pkg.branchPrefix))}</span>`;
            } else if (pkg.branchStatus === 'ahead') {
                cmpBadge = `<span class="badge badge-cat">${escapeHtml(t.statusAhead)}</span>`;
            } else {
                cmpBadge = `<span class="badge badge-idle">${escapeHtml(t.statusDetected)}</span>`;
            }
        } else if (pkg.versionStatus === 'uptodate') {
            cmpBadge = `<span class="badge badge-active">${escapeHtml(t.statusUpToDate)}</span>`;
        } else if (pkg.versionStatus === 'outdated') {
            cmpBadge = `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(pkg.latestVer))}</span>`;
        } else if (pkg.versionStatus === 'ahead') {
            cmpBadge = `<span class="badge badge-cat">${escapeHtml(t.statusAhead)}</span>`;
        } else {
            cmpBadge = `<span class="badge badge-idle">${escapeHtml(t.statusDetected)}</span>`;
        }
    }

    return `
        <div class="version-compare">
            <div class="vc-group">
                <span class="vc-item" title="${escapeHtml(pkg.winActivePath || t.notInPath)}">
                    <span class="vc-label">${escapeHtml(t.activeOnWindows)}</span>
                    ${activeText}
                </span>
                ${versionCompareItems}
            </div>
            ${cmpBadge}
        </div>
    `;
}

function renderToolComparison(tool: tools.ToolStatus): string {
    const t = getT(currentLang());
    const installedText = tool.installed
        ? `<span class="vc-val">${escapeHtml(tool.rawVersion ? `v${tool.rawVersion}` : tool.version)}</span>`
        : `<span class="vc-scope">${escapeHtml(t.badgeNotInstalled)}</span>`;

    const latestText = tool.latestVersion
        ? `<span class="vc-val">v${escapeHtml(tool.latestVersion)}</span>`
        : `<span class="vc-scope">${escapeHtml(t.checkingLabel)}</span>`;

    let badge = `<span class="badge badge-idle">${escapeHtml(t.badgeNotInstalled)}</span>`;
    if (tool.installed) {
        if (tool.versionStatus === 'uptodate') {
            badge = `<span class="badge badge-active">${escapeHtml(t.statusUpToDate)}</span>`;
        } else if (tool.versionStatus === 'outdated') {
            badge = `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(tool.latestVersion))}</span>`;
        } else if (tool.versionStatus === 'ahead') {
            badge = `<span class="badge badge-cat">${escapeHtml(t.statusAhead)}</span>`;
        } else {
            badge = `<span class="badge badge-active">${escapeHtml(t.statusDetected)}</span>`;
        }
    }

    return `
        <div class="version-compare">
            <div class="vc-group">
                <span class="vc-item">
                    <span class="vc-label">${escapeHtml(t.installedLabel)}</span>
                    ${installedText}
                </span>
                <span class="vc-sep">·</span>
                <span class="vc-item">
                    <span class="vc-label">${escapeHtml(t.latestLabel)}</span>
                    ${latestText}
                </span>
            </div>
            ${badge}
        </div>
    `;
}

function renderPackageCard(pkg: main.PackageView): string {
    const lang = currentLang();
    const t = getT(lang);
    const selectedVer = pickDefaultSelectedVersion(pkg);
    selectedVersionByApp[pkg.id] = selectedVer;
    const selectedObj = pkg.versions.find((v) => v.version === selectedVer);
    const isInstalled = Boolean(selectedObj?.installed);
    const isSelectedActive = Boolean(selectedObj?.active);
    const hasUrl = Boolean(selectedObj?.url);

    let statusBadge = `<span class="badge badge-idle">${escapeHtml(t.badgeNotInstalled)}</span>`;
    if (pkg.activeVer) {
        statusBadge = `<span class="badge badge-active">${escapeHtml(t.badgeActiveInPath(pkg.activeVer))}</span>`;
    } else if (pkg.versions.some((v) => v.installed)) {
        statusBadge = `<span class="badge badge-installed">${escapeHtml(t.badgeExtractedOnly)}</span>`;
    }

    const optionsHtml = pkg.versions
        .map((v) => {
            const tags: string[] = [];
            if (v.active) tags.push('Active');
            else if (v.installed) tags.push('Installed');
            const sizePart = v.downloadSize > 0 ? ` (${formatBytes(v.downloadSize)})` : '';
            const suffix = tags.length > 0 ? ` [${tags.join(', ')}]` : '';
            const selectedAttr = v.version === selectedVer ? 'selected' : '';
            return `<option value="${escapeHtml(v.version)}" ${selectedAttr}>v${escapeHtml(v.version)}${escapeHtml(sizePart)}${escapeHtml(suffix)}</option>`;
        })
        .join('');

    const selectedSizeText = selectedObj && selectedObj.downloadSize > 0 ? formatBytes(selectedObj.downloadSize) : '';

    let addPathLabel = t.btnAddToPath;
    if (isSelectedActive) {
        addPathLabel = t.btnActiveInPath;
    } else if (pkg.activeVer) {
        addPathLabel = t.btnSwitchPath;
    }

    const addPathDisabled = busy || !isInstalled || isSelectedActive ? 'disabled' : '';
    const addPathTitle = !isInstalled
        ? t.tipDownloadFirst
        : isSelectedActive
          ? t.tipAlreadyActive
          : t.tipAddToPath;

    const desc = localizePkgDesc(pkg.id, pkg.description || '', lang);

    return `
        <div class="card ${pkg.activeVer ? 'card-active' : ''}" data-app-id="${escapeHtml(pkg.id)}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(pkg.id, pkg.name)}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(pkg.name)}</span>
                            <span class="card-id">[${escapeHtml(pkg.id)}]</span>
                            <span class="badge badge-cat">${escapeHtml(pkg.category === 'stack' ? t.badgeStack : t.badgeCore)}</span>
                        </div>
                        <div class="card-desc">${escapeHtml(desc)}</div>
                    </div>
                </div>
                ${statusBadge}
            </div>

            ${renderVersionComparison(pkg)}

            ${
                pkg.activeDir
                    ? `<div class="path-line"><span>PATH: ${escapeHtml(pkg.activeDir)}</span></div>`
                    : ''
            }
            ${
                pkg.conflict
                    ? `<div class="conflict-banner">⚠ ${escapeHtml(t.pathConflict(pkg.conflict.scope, pkg.conflict.fullPath))}</div>`
                    : ''
            }

            <div class="card-controls">
                <div class="controls-left">
                    <select class="select-box" data-role="ver-select" data-app="${escapeHtml(pkg.id)}">
                        ${optionsHtml || `<option value="">${escapeHtml(t.noVersions)}</option>`}
                    </select>

                    ${
                        selectedSizeText
                            ? `<span class="size-pill" title="${escapeHtml(t.downloadSizeLabel(selectedSizeText))}">↓ ${escapeHtml(selectedSizeText)}</span>`
                            : ''
                    }

                    <button
                        class="btn btn-primary btn-sm"
                        data-action="download"
                        data-app="${escapeHtml(pkg.id)}"
                        ${busy || !hasUrl ? 'disabled' : ''}
                        title="${escapeHtml(hasUrl ? t.btnDownloadTooltip(state?.baseDir || '') : t.btnNoUrlTooltip)}"
                    >
                        ${escapeHtml(isInstalled ? t.btnReDownload : t.btnDownload)}
                    </button>

                    <button
                        class="btn ${isSelectedActive ? 'btn-green' : ''} btn-sm"
                        data-action="add-path"
                        data-app="${escapeHtml(pkg.id)}"
                        ${addPathDisabled}
                        title="${escapeHtml(addPathTitle)}"
                    >
                        ${escapeHtml(addPathLabel)}
                    </button>
                </div>

                <div class="controls-right">
                    ${
                        pkg.activeVer
                            ? `<button class="btn btn-sm" data-action="remove-path" data-app="${escapeHtml(pkg.id)}" ${busy ? 'disabled' : ''} title="${escapeHtml(t.tipRemovePath)}">${escapeHtml(t.btnRemovePath)}</button>`
                            : ''
                    }
                    ${
                        isInstalled
                            ? `<button class="btn btn-danger btn-sm" data-action="delete-ver" data-app="${escapeHtml(pkg.id)}" ${busy ? 'disabled' : ''} title="${escapeHtml(t.tipDelete)}">${escapeHtml(t.btnDelete)}</button>`
                            : ''
                    }
                </div>
            </div>
        </div>
    `;
}

function renderToolSizePill(tool: tools.ToolStatus): string {
    const t = getT(currentLang());
    if (tool.id === 'webview2') {
        if (tool.downloadSize && tool.downloadSize > 0) {
            const sz = formatBytes(tool.downloadSize);
            return `<span class="size-pill" title="${escapeHtml(t.toolSizeWebView2Tip)}">↓ ${escapeHtml(sz)}</span>`;
        }
        return '';
    }
    if (tool.id === 'vsbuildtools') {
        const bootSz = tool.downloadSize && tool.downloadSize > 0 ? formatBytes(tool.downloadSize) : '';
        return `<span class="size-pill size-pill-warn" title="${escapeHtml(t.toolSizeVSBuildTip)}">↓ ${escapeHtml(t.toolSizeVSBuild(bootSz))}</span>`;
    }
    if (tool.id === 'wails2' || tool.id === 'wails3') {
        return `<span class="size-pill size-pill-muted" title="${escapeHtml(t.toolSizeGoDynamicTip)}">${escapeHtml(t.toolSizeGoDynamic)}</span>`;
    }
    if (tool.id === 'tauri') {
        return `<span class="size-pill size-pill-warn" title="${escapeHtml(t.toolSizeCargoHeavyTip)}">${escapeHtml(t.toolSizeCargoHeavy)}</span>`;
    }
    const pkgMgrPills: Record<string, [string, string]> = {
        scoop: ['User-space (PowerShell)', 'size-pill-muted'],
        choco: ['Admin (UAC)', 'size-pill-warn'],
        npm: ['npm install -g', 'size-pill-muted'],
        pnpm: ['npm / ~40 MB (.exe)', 'size-pill-muted'],
        bun: ['↓ ~40 MB (.zip)', ''],
        uv: ['↓ ~16 MB (.zip)', ''],
        pip: ['pip install -U', 'size-pill-muted'],
        cargo: ['cargo install', 'size-pill-muted'],
        dotnet: ['dotnet tool -g', 'size-pill-muted'],
        psgallery: ['Install-PSResource', 'size-pill-muted'],
    };
    const pillSpec = pkgMgrPills[tool.id];
    if (pillSpec) {
        return `<span class="size-pill ${pillSpec[1]}">${escapeHtml(pillSpec[0])}</span>`;
    }
    return '';
}

function renderToolCard(tool: tools.ToolStatus): string {
    const lang = currentLang();
    const t = getT(lang);
    const statusBadge = tool.installed
        ? `<span class="badge badge-active">${escapeHtml(tool.version || 'Installed')}</span>`
        : `<span class="badge badge-idle">${escapeHtml(t.badgeNotInstalled)}</span>`;

    const desc = localizeToolDesc(tool.id, tool.description, lang);
    const req = localizeToolRequires(tool.id, tool.requires, lang);
    const hasUpdate = tool.installed && tool.versionStatus === 'outdated';
    const updateTip = !tool.latestVersion
        ? t.tipCheckingUpdate
        : hasUpdate
          ? t.tipUpdateAvailable(tool.latestVersion)
          : t.tipNoUpdate;

    const actionButtonsHtml = !tool.installed
        ? `
            <button
                class="btn btn-primary btn-sm"
                data-action="install-tool"
                data-tool="${escapeHtml(tool.id)}"
                data-mode="install"
                ${busy ? 'disabled' : ''}
            >
                ${escapeHtml(t.btnInstallCmd)}
            </button>
        `
        : `
            <button
                class="btn ${hasUpdate ? 'btn-primary' : ''} btn-sm"
                data-action="install-tool"
                data-tool="${escapeHtml(tool.id)}"
                data-mode="update"
                ${busy || !hasUpdate ? 'disabled' : ''}
                title="${escapeHtml(updateTip)}"
            >
                ${escapeHtml(t.btnUpdateTool)}
            </button>
            <button
                class="btn btn-sm"
                data-action="install-tool"
                data-tool="${escapeHtml(tool.id)}"
                data-mode="fix"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.tipFixTool)}"
            >
                ${escapeHtml(t.btnFixTool)}
            </button>
        `;

    const badgeLabel =
        tool.category === 'sdk' ? t.badgeSDK : tool.category === 'pkgmgr' ? t.badgePkgMgr : t.badgeCLI;

    return `
        <div class="card ${tool.installed ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(tool.id, tool.name)}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(tool.name)}</span>
                            <span class="badge badge-cat">${escapeHtml(badgeLabel)}</span>
                            ${req ? `<span class="card-id">${escapeHtml(req)}</span>` : ''}
                        </div>
                        <div class="card-desc">${escapeHtml(desc)}</div>
                    </div>
                </div>
                ${statusBadge}
            </div>

            ${renderToolComparison(tool)}

            ${
                tool.location
                    ? `<div class="path-line"><span>${escapeHtml(t.locationLabel)}: ${escapeHtml(tool.location)}</span></div>`
                    : ''
            }

            <div class="cmd-hint">$ ${escapeHtml(tool.commandHint)}</div>

            <div class="card-controls">
                <div class="controls-left">
                    ${actionButtonsHtml}
                    ${renderToolSizePill(tool)}
                    ${
                        tool.installed && !tool.inPath && tool.binDir
                            ? `<button class="btn btn-green btn-sm" data-action="add-custom-path" data-dir="${escapeHtml(tool.binDir)}" ${busy ? 'disabled' : ''}>${escapeHtml(t.btnAddDirToPath(tool.binDir))}</button>`
                            : ''
                    }
                </div>
            </div>
        </div>
    `;
}

function renderEssentialCard(item: main.EssentialView): string {
    const lang = currentLang();
    const t = getT(lang);
    const isChecked = Boolean(checkedEssentials[item.id]);
    const selectedVer = pickDefaultEssentialVersion(item);
    selectedEssentialVerByApp[item.id] = selectedVer;
    const desc = localizeEssentialDesc(item.id, item.description || '', lang);
    const det = getCachedDetails(item.wingetId, item.source);
    const curated = getCuratedMeta(item.id);
    const metaStrip = renderMetaStrip({
        publisher: det?.publisher || curated?.publisher,
        domain: shortDomain(det?.homepage || det?.publisherUrl) || curated?.domain,
        license: det?.license || curated?.license,
        installerType: det?.installerType,
    });

    const statusBadge = item.installed
        ? item.updateIgnored
            ? `<span class="badge badge-active">${escapeHtml(item.installedVer ? t.essBadgeInstalled(item.installedVer) : t.essBadgeInstalledNoVer)}</span>`
            : item.hasUpdate
              ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(item.availableVer))}</span>`
              : item.installedVer
                ? `<span class="badge badge-active">${escapeHtml(t.essBadgeInstalled(item.installedVer))}</span>`
                : `<span class="badge badge-active">${escapeHtml(t.essBadgeInstalledNoVer)}</span>`
        : `<span class="badge badge-idle">${escapeHtml(t.badgeNotInstalled)}</span>`;

    const versionDropdownHtml =
        item.versions && item.versions.length > 0
            ? `
                <select class="select-box" style="min-width:140px" data-role="ess-ver-select" data-ess="${escapeHtml(item.id)}">
                    ${item.versions
                        .map((v) => {
                            const label =
                                item.id === 'laragon' && v === '6.0.0'
                                    ? t.essLaragonNoPopupNote
                                    : v === 'latest'
                                      ? 'Latest'
                                      : `v${v}`;
                            const sel = v === selectedVer ? 'selected' : '';
                            return `<option value="${escapeHtml(v)}" ${sel}>${escapeHtml(label)}</option>`;
                        })
                        .join('')}
                </select>
            `
            : '';

    const updateTip = item.updateIgnored
        ? t.essBadgeUpdateIgnored
        : item.hasUpdate
          ? t.tipUpdateAvailable(item.availableVer)
          : t.tipNoUpdate;
    const wingetMissing = !state?.wingetEnv?.wingetAvailable;

    const ignoreControlBtn = item.updateIgnored
        ? `
            <button
                class="btn btn-sm btn-restore"
                data-action="winget-unignore"
                data-wid="${escapeHtml(item.wingetId)}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnRestoreTip)}"
            >
                ${escapeHtml(t.essBtnRestore)}
            </button>
        `
        : item.pauseUntil
          ? `
            <button
                class="btn btn-sm btn-restore"
                data-action="winget-unignore"
                data-wid="${escapeHtml(item.wingetId)}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.pkgBtnResumeUpdatesTip)}"
            >
                ${escapeHtml(t.pkgBtnResumeUpdates)}
            </button>
        `
          : item.skippedVersion
            ? `
            <button
                class="btn btn-sm btn-restore"
                data-action="winget-unignore"
                data-wid="${escapeHtml(item.wingetId)}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.pkgBtnClearSkipTip)}"
            >
                ${escapeHtml(t.pkgBtnClearSkip)}
            </button>
        `
            : item.hasUpdate
              ? `
            <button
                class="btn btn-sm"
                data-action="winget-ignore"
                data-name="${escapeHtml(item.name)}"
                data-wid="${escapeHtml(item.wingetId)}"
                data-source="${escapeHtml(item.source || 'winget')}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnIgnoreTip)}"
            >
                ${escapeHtml(t.essBtnIgnore)}
            </button>
        `
              : '';

    const actionButtonsHtml = !item.installed
        ? `
            <button
                class="btn btn-primary btn-sm"
                data-action="ess-run"
                data-ess="${escapeHtml(item.id)}"
                data-mode="install"
                ${busy || wingetMissing ? 'disabled' : ''}
            >
                ${escapeHtml(t.essInstallSilent)}
            </button>
        `
        : `
            <button
                class="btn ${item.hasUpdate ? 'btn-primary' : ''} btn-sm"
                data-action="ess-run"
                data-ess="${escapeHtml(item.id)}"
                data-mode="update"
                ${busy || wingetMissing || !item.hasUpdate ? 'disabled' : ''}
                title="${escapeHtml(updateTip)}"
            >
                ${escapeHtml(t.btnUpdateTool)}
            </button>
            <button
                class="btn btn-sm"
                data-action="ess-run"
                data-ess="${escapeHtml(item.id)}"
                data-mode="fix"
                ${busy || wingetMissing ? 'disabled' : ''}
                title="${escapeHtml(t.tipFixTool)}"
            >
                ${escapeHtml(t.btnFixTool)}
            </button>
            ${ignoreControlBtn}
        `;

    return `
        <div class="card ${item.installed ? 'card-active' : ''} ${isChecked ? 'card-selected' : ''}">
            <div class="card-top">
                <div class="card-check-wrap card-main-row">
                    <input
                        class="app-checkbox"
                        type="checkbox"
                        data-role="ess-check"
                        data-ess="${escapeHtml(item.id)}"
                        ${isChecked ? 'checked' : ''}
                        ${busy ? 'disabled' : ''}
                    />
                    ${renderAppIcon(item.id, item.name, { wingetId: item.wingetId, dataUrl: det?.iconDataUrl, iconUrl: item.iconUrl || det?.iconUrl })}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(item.name)}</span>
                            <span class="card-id">[${escapeHtml(item.wingetId)}]</span>
                            <span class="badge ${item.source === 'msstore' ? 'badge-store' : 'badge-cat'}">
                                ${escapeHtml(item.source === 'msstore' ? t.essBadgeMsStore : 'winget')}
                            </span>
                            ${item.requiresUac ? `<span class="badge badge-warn">${escapeHtml(t.essBadgeUAC)}</span>` : ''}
                        </div>
                        <div class="card-desc">${escapeHtml(desc)}</div>
                        ${metaStrip}
                    </div>
                </div>
                ${statusBadge}
            </div>

            ${
                item.installed
                    ? `
                    <div class="version-compare">
                        <div class="vc-group">
                            <span class="vc-item" ${item.matchedBy ? `title="${escapeHtml(item.matchedBy)}"` : ''}>
                                <span class="vc-label">${escapeHtml(t.installedLabel)}</span>
                                <span class="vc-val">${escapeHtml(item.installedVer ? `v${item.installedVer}` : t.statusDetected)}</span>
                            </span>
                            ${
                                item.availableVer
                                    ? `
                                    <span class="vc-sep">·</span>
                                    <span class="vc-item">
                                        <span class="vc-label">${escapeHtml(t.essWingetLatestLabel)}</span>
                                        <span class="vc-val">v${escapeHtml(item.availableVer)}</span>
                                    </span>
                                `
                                    : ''
                            }
                        </div>
                        ${
                            item.updateIgnored
                                ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeUpdateIgnored)}</span>`
                                : item.pauseUntil
                                  ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgePausedUntil(item.pauseUntil))}</span>`
                                  : item.skippedVersion
                                    ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgeSkipped(item.skippedVersion))}</span>`
                                    : item.hasUpdate
                                      ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(item.availableVer))}</span>`
                                      : item.versionStatus === 'ahead'
                                        ? `<span class="badge badge-cat">${escapeHtml(t.essStatusNewerThanWinget)}</span>`
                                        : `<span class="badge badge-active">${escapeHtml(item.installedVer ? t.statusUpToDate : t.statusDetected)}</span>`
                        }
                    </div>
                `
                    : ''
            }

            <div class="card-controls">
                <div class="controls-left">
                    ${versionDropdownHtml}
                    ${
                        (item.downloadSize || det?.installerSize || 0) > 0
                            ? `<span class="size-pill" title="${escapeHtml(t.downloadSizeLabel(formatBytes(item.downloadSize || det?.installerSize || 0)))}">↓ ${escapeHtml(formatBytes(item.downloadSize || det?.installerSize || 0))}</span>`
                            : ''
                    }
                    ${actionButtonsHtml}
                </div>
                <div class="controls-right">
                    <button
                        class="btn btn-sm"
                        data-action="ess-detail"
                        data-mgr="winget"
                        data-ess="${escapeHtml(item.id)}"
                        data-name="${escapeHtml(item.name)}"
                        data-wid="${escapeHtml(item.wingetId)}"
                        data-source="${escapeHtml(item.source || 'winget')}"
                        data-ver="${escapeHtml(item.installedVer || selectedVer)}"
                        data-desc="${escapeHtml(desc)}"
                        data-iconurl="${escapeHtml(item.iconUrl || '')}"
                        data-isess="1"
                    >
                        ${escapeHtml(t.essDetailsBtn)}
                    </button>
                </div>
            </div>
        </div>
    `;
}

function managerDisplayName(mgr: string): string {
    switch ((mgr || 'winget').toLowerCase()) {
        case 'scoop':
            return 'Scoop';
        case 'choco':
            return 'Chocolatey';
        case 'npm':
            return 'npm';
        case 'pnpm':
            return 'pnpm';
        case 'bun':
            return 'Bun';
        case 'uv':
            return 'uv (PyPI)';
        case 'pip':
            return 'pip';
        case 'cargo':
            return 'Cargo';
        case 'dotnet':
            return '.NET Tool';
        case 'psgallery':
            return 'PSGallery';
        default:
            return 'Winget';
    }
}

function renderManagerBadge(mgr: string, source?: string): string {
    const norm = (mgr || 'winget').toLowerCase();
    if (norm === 'winget') {
        return `<span class="badge ${source === 'msstore' ? 'badge-store' : 'badge-cat'}">${escapeHtml(source || 'winget')}</span>`;
    }
    const label =
        norm === 'scoop' && source && source !== 'scoop'
            ? `scoop · ${source}`
            : norm === 'uv'
              ? 'uv · pypi'
              : norm === 'pip'
                ? 'pip · pypi'
                : norm === 'cargo'
                  ? 'cargo · crates.io'
                  : norm === 'dotnet'
                    ? '.net · nuget'
                    : norm === 'psgallery'
                      ? 'psgallery'
                      : norm;
    return `<span class="badge badge-mgr-${escapeHtml(norm)}">${escapeHtml(label)}</span>`;
}

function isExtraManagerAvailable(mgr: string): boolean {
    if (mgr === 'winget') return Boolean(state?.wingetEnv?.wingetAvailable);
    return Boolean((state?.pkgManagers || []).find((m) => m.id === mgr)?.available);
}

function renderExtraSearchCard(
    row: pkgmgr.PackageEntry,
    installedByKey: Map<string, main.InstalledAppView>,
    targetMgrOptions?: ManagerKey[],
    targetGroupKey?: string,
): string {
    const t = getT(currentLang());
    const idLower = row.id.toLowerCase();
    const rawMgr = (row.manager || 'scoop').toLowerCase() as ManagerKey;
    const isMultiTargetChoice = Boolean(targetMgrOptions && targetMgrOptions.length > 1);
    const selectKey = `${targetGroupKey || rawMgr}:${idLower}`;
    const liveInst =
        isMultiTargetChoice && targetMgrOptions
            ? targetMgrOptions.map((jm) => installedByKey.get(`${jm}:${idLower}`)).find(Boolean)
            : installedByKey.get(`${rawMgr}:${idLower}`);

    const mgr: ManagerKey =
        ((liveInst?.manager ||
            (row.installed ? row.manager : '') ||
            (isMultiTargetChoice ? selectedSearchJSMgrByPkg[selectKey] || targetMgrOptions![0] : rawMgr)).toLowerCase() as ManagerKey) ||
        'scoop';

    const isInstalled = Boolean(liveInst || row.installed);
    const installedVer = liveInst?.installedVer || row.installedVer || '';
    const isIgnored = Boolean(liveInst?.ignored);
    const availVer = isIgnored
        ? ''
        : liveInst?.hasUpdate
          ? liveInst.availableVer
          : row.hasUpdate
            ? row.availableVer || row.version
            : '';
    const hasUpdate = Boolean(availVer);
    const mgrMissing = !isExtraManagerAvailable(mgr);
    const canUninstall =
        !(mgr === 'choco' && idLower === 'chocolatey') &&
        !(mgr === 'npm' && idLower === 'npm') &&
        !(mgr === 'pip' && (idLower === 'pip' || idLower === 'setuptools' || idLower === 'wheel'));

    const statusBadge = isInstalled
        ? isIgnored
            ? `<span class="badge badge-active">${escapeHtml(installedVer ? t.essBadgeInstalled(installedVer) : t.essBadgeInstalledNoVer)}</span>`
            : hasUpdate
              ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(availVer))}</span>`
              : `<span class="badge badge-active">${escapeHtml(installedVer ? t.essBadgeInstalled(installedVer) : t.essBadgeInstalledNoVer)}</span>`
        : '';

    const versionCompareHtml = isInstalled
        ? `
            <div class="version-compare">
                <div class="vc-group">
                    <span class="vc-item">
                        <span class="vc-label">${escapeHtml(t.installedLabel)}</span>
                        <span class="vc-val">${escapeHtml(installedVer ? `v${installedVer}` : t.statusDetected)}</span>
                    </span>
                    ${
                        row.version
                            ? `
                            <span class="vc-sep">·</span>
                            <span class="vc-item">
                                <span class="vc-label">${escapeHtml(t.latestLabel)}</span>
                                <span class="vc-val">v${escapeHtml(row.version)}</span>
                            </span>
                        `
                            : ''
                    }
                </div>
                ${
                    isIgnored
                        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeUpdateIgnored)}</span>`
                        : hasUpdate
                          ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(availVer))}</span>`
                          : `<span class="badge badge-active">${escapeHtml(installedVer ? t.statusUpToDate : t.statusDetected)}</span>`
                }
            </div>
        `
        : '';

    const jsTargetSelectHtml =
        !isInstalled && isMultiTargetChoice && targetMgrOptions
            ? `
                <select
                    class="select-box select-box-compact"
                    data-role="search-js-mgr"
                    data-pkg-key="${escapeHtml(selectKey)}"
                    title="${escapeHtml(t.pkgSearchInstallWithTip)}"
                    ${busy ? 'disabled' : ''}
                >
                    ${targetMgrOptions
                        .map(
                            (jm) =>
                                `<option value="${escapeHtml(jm)}" ${jm === mgr ? 'selected' : ''}>${escapeHtml(managerDisplayName(jm))}</option>`,
                        )
                        .join('')}
                </select>
            `
            : '';

    const actionHtml = isInstalled
        ? `
            ${
                hasUpdate
                    ? `
                    <button
                        class="btn btn-primary btn-sm"
                        data-action="pkg-action"
                        data-mgr="${escapeHtml(mgr)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-name="${escapeHtml(row.name)}"
                        data-source="${escapeHtml(row.source || '')}"
                        data-mode="update"
                        ${busy || mgrMissing ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.btnUpdateTool)}
                    </button>
                `
                    : ''
            }
            <button
                class="btn btn-sm"
                data-action="pkg-action"
                data-mgr="${escapeHtml(mgr)}"
                data-wid="${escapeHtml(row.id)}"
                data-name="${escapeHtml(row.name)}"
                data-source="${escapeHtml(row.source || '')}"
                data-mode="install"
                ${busy || mgrMissing ? 'disabled' : ''}
            >
                ${escapeHtml(t.essReinstallSilent)}
            </button>
            ${
                canUninstall
                    ? `
                    <button
                        class="btn btn-danger btn-sm"
                        data-action="pkg-action"
                        data-mgr="${escapeHtml(mgr)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-name="${escapeHtml(row.name)}"
                        data-source="${escapeHtml(row.source || '')}"
                        data-mode="uninstall"
                        ${busy || mgrMissing ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnUninstallTip)}"
                    >
                        ${escapeHtml(t.pkgBtnUninstall)}
                    </button>
                `
                    : ''
            }
        `
        : `
            ${jsTargetSelectHtml}
            <button
                class="btn btn-primary btn-sm"
                data-action="pkg-action"
                data-mgr="${escapeHtml(mgr)}"
                data-wid="${escapeHtml(row.id)}"
                data-name="${escapeHtml(row.name)}"
                data-source="${escapeHtml(row.source || '')}"
                data-mode="install"
                ${busy || mgrMissing ? 'disabled' : ''}
                title="${escapeHtml(mgrMissing ? t.pkgMgrNotReady(managerDisplayName(mgr)) : '')}"
            >
                ${escapeHtml(t.essInstallSilent)}
            </button>
        `;

    return `
        <div class="card ${isInstalled ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(row.id, row.name, { iconUrl: row.iconUrl })}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(row.name)}</span>
                            <span class="card-id">[${escapeHtml(row.id)}]</span>
                            ${renderManagerBadge(mgr, row.source)}
                            ${row.version ? `<span class="badge badge-idle">v${escapeHtml(row.version)}</span>` : ''}
                        </div>
                        ${
                            row.description
                                ? `<div class="card-desc card-desc-clamp">${escapeHtml(row.description)}</div>`
                                : `<div class="card-desc">${escapeHtml(t.essVersionLabel)} ${escapeHtml(row.version || '-')}</div>`
                        }
                    </div>
                </div>
                ${statusBadge}
            </div>
            ${versionCompareHtml}
            <div class="card-controls">
                <div class="controls-left">
                    ${actionHtml}
                    <button
                        class="btn btn-sm"
                        data-action="ess-detail"
                        data-mgr="${escapeHtml(mgr)}"
                        data-name="${escapeHtml(row.name)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-source="${escapeHtml(row.source || mgr)}"
                        data-ver="${escapeHtml(row.version || '')}"
                        data-desc="${escapeHtml(row.description || '')}"
                        data-iconurl="${escapeHtml(row.iconUrl || '')}"
                        data-isess="0"
                    >
                        ${escapeHtml(t.essDetailsBtn)}
                    </button>
                    ${
                        row.homepage
                            ? `<button class="btn btn-sm" data-open-url="${escapeHtml(row.homepage)}">${escapeHtml(t.essOpenHomepage)}</button>`
                            : ''
                    }
                </div>
            </div>
        </div>
    `;
}

function searchSourceFilterLabel(key: SearchFilterKey, jsMgrs: ManagerKey[], pyMgrs: ManagerKey[]): string {
    switch (key) {
        case 'winget':
            return 'Winget';
        case 'msstore':
            return 'MS Store';
        case 'scoop':
            return 'Scoop';
        case 'choco':
            return 'Chocolatey';
        case 'js':
            return jsMgrs.length > 1 ? jsMgrs.map(managerDisplayName).join(' / ') : managerDisplayName(jsMgrs[0] || 'npm');
        case 'py':
            return pyMgrs.length > 1 ? pyMgrs.map(managerDisplayName).join(' / ') : managerDisplayName(pyMgrs[0] || 'uv');
        case 'cargo':
            return 'Cargo';
        case 'dotnet':
            return '.NET Tool';
        case 'psgallery':
            return 'PSGallery';
        default:
            return 'All';
    }
}

function searchPendingGroupLabel(group: string, jsMgrs: ManagerKey[], pyMgrs: ManagerKey[]): string {
    switch (group) {
        case 'winget':
            return 'Winget / MS Store';
        case 'scoop':
            return 'Scoop';
        case 'choco':
            return 'Chocolatey';
        case 'js':
            return jsMgrs.length > 1 ? jsMgrs.map(managerDisplayName).join('/') : managerDisplayName(jsMgrs[0] || 'npm');
        case 'py':
            return pyMgrs.length > 1 ? pyMgrs.map(managerDisplayName).join('/') : managerDisplayName(pyMgrs[0] || 'uv');
        case 'cargo':
            return 'Cargo';
        case 'dotnet':
            return '.NET Tool';
        case 'psgallery':
            return 'PSGallery';
        default:
            return group;
    }
}

function searchMatchScore(queryLower: string, name: string, id: string): number {
    if (!queryLower) return 0;
    const n = name.toLowerCase();
    const idL = id.toLowerCase();
    if (idL === queryLower || n === queryLower) return 3;
    if (n.startsWith(queryLower) || idL.startsWith(queryLower)) return 2;
    if (n.includes(queryLower) || idL.includes(queryLower)) return 1;
    return 0;
}

function renderWingetSearchCard(
    row: essentials.WingetTableRow,
    pinnedIDs: Set<string>,
    installedWingetByID: Map<string, main.InstalledAppView>,
    curatedByID: Map<string, main.EssentialView>,
    wingetMissing: boolean,
): string {
    const t = getT(currentLang());
    const idLower = row.id.toLowerCase();
    const isPinned = pinnedIDs.has(idLower);
    const liveInst = installedWingetByID.get(idLower);
    const liveEss = curatedByID.get(idLower);

    const isInstalled = Boolean(liveInst || liveEss?.installed || row.installed);
    const installedVer =
        liveInst?.installedVer || (liveEss?.installed ? liveEss.installedVer : '') || row.installedVer || '';
    const isIgnored = Boolean(liveInst?.ignored || liveEss?.updateIgnored);
    const availVer = isIgnored
        ? ''
        : liveInst?.hasUpdate
          ? liveInst.availableVer
          : liveEss?.hasUpdate
            ? liveEss.availableVer
            : row.hasUpdate
              ? row.available || row.version
              : '';
    const hasUpdate = Boolean(availVer);
    const matchedBy = liveEss?.matchedBy || row.matchedBy || (liveInst ? 'Winget' : '');

    const det = getCachedDetails(row.id, row.source);
    const domain = shortDomain(det?.homepage || det?.publisherUrl || det?.installerUrl);
    const metaStrip = renderMetaStrip({
        publisher: det?.publisher,
        domain,
        license: det?.license,
        installerType: det?.installerType,
        match: row.match,
    });
    const descText = det?.description
        ? `<div class="card-desc card-desc-clamp">${escapeHtml(det.description)}</div>`
        : `<div class="card-desc">${escapeHtml(t.essVersionLabel)} ${escapeHtml(row.version || '-')}</div>`;

    const statusBadge = isInstalled
        ? isIgnored
            ? `<span class="badge badge-active">${escapeHtml(installedVer ? t.essBadgeInstalled(installedVer) : t.essBadgeInstalledNoVer)}</span>`
            : hasUpdate
              ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(availVer))}</span>`
              : installedVer
                ? `<span class="badge badge-active">${escapeHtml(t.essBadgeInstalled(installedVer))}</span>`
                : `<span class="badge badge-active">${escapeHtml(t.essBadgeInstalledNoVer)}</span>`
        : '';

    const versionCompareHtml = isInstalled
        ? `
            <div class="version-compare">
                <div class="vc-group">
                    <span class="vc-item" ${matchedBy ? `title="${escapeHtml(matchedBy)}"` : ''}>
                        <span class="vc-label">${escapeHtml(t.installedLabel)}</span>
                        <span class="vc-val">${escapeHtml(installedVer ? `v${installedVer}` : t.statusDetected)}</span>
                    </span>
                    ${
                        row.version
                            ? `
                            <span class="vc-sep">·</span>
                            <span class="vc-item">
                                <span class="vc-label">${escapeHtml(t.essWingetLatestLabel)}</span>
                                <span class="vc-val">v${escapeHtml(row.version)}</span>
                            </span>
                        `
                            : ''
                    }
                </div>
                ${
                    isIgnored
                        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeUpdateIgnored)}</span>`
                        : hasUpdate
                          ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(availVer))}</span>`
                          : `<span class="badge badge-active">${escapeHtml(installedVer ? t.statusUpToDate : t.statusDetected)}</span>`
                }
            </div>
        `
        : '';

    const installOrUpdateBtn = isInstalled
        ? `
            ${
                hasUpdate
                    ? `
                    <button
                        class="btn btn-primary btn-sm"
                        data-action="winget-update-raw"
                        data-name="${escapeHtml(row.name)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-source="${escapeHtml(row.source || 'winget')}"
                        ${busy || wingetMissing ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.btnUpdateTool)}
                    </button>
                `
                    : ''
            }
            <button
                class="btn btn-sm"
                data-action="winget-install-raw"
                data-name="${escapeHtml(row.name)}"
                data-wid="${escapeHtml(row.id)}"
                data-source="${escapeHtml(row.source || 'winget')}"
                ${busy || wingetMissing ? 'disabled' : ''}
            >
                ${escapeHtml(t.essReinstallSilent)}
            </button>
        `
        : `
            <button
                class="btn btn-primary btn-sm"
                data-action="winget-install-raw"
                data-name="${escapeHtml(row.name)}"
                data-wid="${escapeHtml(row.id)}"
                data-source="${escapeHtml(row.source || 'winget')}"
                ${busy || wingetMissing ? 'disabled' : ''}
            >
                ${escapeHtml(t.essInstallSilent)}
            </button>
        `;

    return `
        <div class="card ${isInstalled ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(row.id, row.name, { wingetId: row.id, dataUrl: det?.iconDataUrl, iconUrl: det?.iconUrl || row.iconUrl })}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(det?.name || row.name)}</span>
                            <span class="card-id">[${escapeHtml(row.id)}]</span>
                            <span class="badge ${row.source === 'msstore' ? 'badge-store' : 'badge-cat'}">
                                ${escapeHtml(row.source || 'winget')}
                            </span>
                            ${row.version ? `<span class="badge badge-idle">v${escapeHtml(row.version)}</span>` : ''}
                        </div>
                        ${descText}
                        ${metaStrip}
                    </div>
                </div>
                ${statusBadge}
            </div>
            ${versionCompareHtml}
            <div class="card-controls">
                <div class="controls-left">
                    ${installOrUpdateBtn}
                    <button
                        class="btn btn-sm"
                        data-action="winget-pin"
                        data-name="${escapeHtml(row.name)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-source="${escapeHtml(row.source || 'winget')}"
                        ${busy || isPinned ? 'disabled' : ''}
                    >
                        ${escapeHtml(isPinned ? t.essAlreadyPinned : t.essPinToList)}
                    </button>
                    <button
                        class="btn btn-sm"
                        data-action="ess-detail"
                        data-mgr="winget"
                        data-name="${escapeHtml(det?.name || row.name)}"
                        data-wid="${escapeHtml(row.id)}"
                        data-source="${escapeHtml(row.source || 'winget')}"
                        data-ver="${escapeHtml(row.version || '')}"
                        data-desc="${escapeHtml(det?.description || '')}"
                        data-iconurl="${escapeHtml(det?.iconUrl || row.iconUrl || '')}"
                        data-isess="0"
                    >
                        ${escapeHtml(t.essDetailsBtn)}
                    </button>
                    ${
                        det?.installerSize && det.installerSize > 0
                            ? `<span class="size-pill" title="${escapeHtml(t.downloadSizeLabel(formatBytes(det.installerSize)))}">↓ ${escapeHtml(formatBytes(det.installerSize))}</span>`
                            : ''
                    }
                </div>
            </div>
        </div>
    `;
}

function renderWingetSearchView(): string {
    const t = getT(currentLang());
    const pinnedIDs = new Set((state?.essentials || []).map((e) => e.wingetId.toLowerCase()));
    const installedWingetByID = new Map(
        (state?.installedApps || [])
            .filter((a) => (a.manager || 'winget') === 'winget')
            .map((a) => [a.id.toLowerCase(), a]),
    );
    const installedAllByKey = new Map(
        (state?.installedApps || []).map((a) => [`${(a.manager || 'winget').toLowerCase()}:${a.id.toLowerCase()}`, a]),
    );
    const curatedByID = new Map((state?.essentials || []).map((e) => [e.wingetId.toLowerCase(), e]));
    const wingetMissing = !state?.wingetEnv?.wingetAvailable;
    const anyPkgMgrAvailable =
        !wingetMissing || (state?.pkgManagers || []).some((m) => m.available);

    const wingetOnlyRows = wingetSearchResults.filter((r) => (r.source || 'winget').toLowerCase() !== 'msstore');
    const msstoreRows = wingetSearchResults.filter((r) => (r.source || '').toLowerCase() === 'msstore');
    const extraSpecs: Array<{
        filterKey: SearchFilterKey;
        groupKey: string;
        groupOrder: number;
        rows: pkgmgr.PackageEntry[];
        altMgrs?: ManagerKey[];
    }> = [
        { filterKey: 'scoop', groupKey: 'scoop', groupOrder: 2, rows: extraSearchByGroup['scoop'] || [] },
        { filterKey: 'choco', groupKey: 'choco', groupOrder: 3, rows: extraSearchByGroup['choco'] || [] },
        { filterKey: 'js', groupKey: 'js', groupOrder: 4, rows: extraSearchByGroup['js'] || [], altMgrs: unifiedJSMgrs },
        { filterKey: 'py', groupKey: 'py', groupOrder: 5, rows: extraSearchByGroup['py'] || [], altMgrs: unifiedPyMgrs },
        { filterKey: 'cargo', groupKey: 'cargo', groupOrder: 6, rows: extraSearchByGroup['cargo'] || [] },
        { filterKey: 'dotnet', groupKey: 'dotnet', groupOrder: 7, rows: extraSearchByGroup['dotnet'] || [] },
        { filterKey: 'psgallery', groupKey: 'psgallery', groupOrder: 8, rows: extraSearchByGroup['psgallery'] || [] },
    ];

    const sourceCounts: Record<SearchFilterKey, number> = {
        all: 0,
        winget: wingetOnlyRows.length,
        msstore: msstoreRows.length,
        scoop: 0,
        choco: 0,
        js: 0,
        py: 0,
        cargo: 0,
        dotnet: 0,
        psgallery: 0,
    };
    let totalResults = sourceCounts.winget + sourceCounts.msstore;
    for (const sp of extraSpecs) {
        sourceCounts[sp.filterKey] = sp.rows.length;
        totalResults += sp.rows.length;
    }
    sourceCounts.all = totalResults;

    const filterOrder: SearchFilterKey[] = ['winget', 'msstore', 'scoop', 'choco', 'js', 'py', 'cargo', 'dotnet', 'psgallery'];
    const populatedFilters = filterOrder.filter((k) => sourceCounts[k] > 0);
    const effectiveFilter: SearchFilterKey =
        searchResultFilter !== 'all' && sourceCounts[searchResultFilter] > 0 ? searchResultFilter : 'all';

    // Only render the horizontal filter bar once at least one package manager has returned results.
    const filterRowHtml =
        totalResults > 0
            ? `
            <div class="mgr-filter-row">
                <button class="mgr-chip ${effectiveFilter === 'all' ? 'active' : ''}" data-search-filter="all">
                    <span>${escapeHtml(t.pkgSearchFilterAll(totalResults))}</span>
                </button>
                ${populatedFilters
                    .map(
                        (k) => `
                        <button class="mgr-chip ${effectiveFilter === k ? 'active' : ''}" data-search-filter="${k}">
                            <span>${escapeHtml(searchSourceFilterLabel(k, unifiedJSMgrs, unifiedPyMgrs))}</span>
                            <span class="mgr-chip-count">${sourceCounts[k]}</span>
                        </button>
                    `,
                    )
                    .join('')}
            </div>
        `
            : '';

    const pendingBannerHtml =
        wingetSearching && unifiedSearchPending.length > 0
            ? `
            <div class="search-progress-pill">
                <span class="spinner-dot"></span>
                <span>${escapeHtml(t.pkgSearchPendingNote(unifiedSearchPending.map((g) => searchPendingGroupLabel(g, unifiedJSMgrs, unifiedPyMgrs))))}</span>
            </div>
        `
            : '';

    const qLower = wingetCatalogQuery.trim().toLowerCase();
    interface SearchCardSlot {
        groupOrder: number;
        score: number;
        index: number;
        html: string;
    }
    const cardSlots: SearchCardSlot[] = [];

    if (effectiveFilter === 'all' || effectiveFilter === 'winget') {
        wingetOnlyRows.forEach((row, idx) => {
            cardSlots.push({
                groupOrder: 0,
                score: searchMatchScore(qLower, row.name, row.id),
                index: idx,
                html: renderWingetSearchCard(row, pinnedIDs, installedWingetByID, curatedByID, wingetMissing),
            });
        });
    }
    if (effectiveFilter === 'all' || effectiveFilter === 'msstore') {
        msstoreRows.forEach((row, idx) => {
            cardSlots.push({
                groupOrder: 1,
                score: searchMatchScore(qLower, row.name, row.id),
                index: idx,
                html: renderWingetSearchCard(row, pinnedIDs, installedWingetByID, curatedByID, wingetMissing),
            });
        });
    }
    for (const sp of extraSpecs) {
        if (effectiveFilter !== 'all' && effectiveFilter !== sp.filterKey) continue;
        sp.rows.forEach((row, idx) => {
            cardSlots.push({
                groupOrder: sp.groupOrder,
                score: searchMatchScore(qLower, row.name, row.id),
                index: idx,
                html: renderExtraSearchCard(row, installedAllByKey, sp.altMgrs, sp.groupKey),
            });
        });
    }

    if (effectiveFilter === 'all' && cardSlots.length > 1) {
        cardSlots.sort((a, b) => {
            if (a.score !== b.score) return b.score - a.score;
            if (a.groupOrder !== b.groupOrder) return a.groupOrder - b.groupOrder;
            return a.index - b.index;
        });
    }

    let bodyHtml = `<div class="empty-state">${escapeHtml(t.essSearchEmpty)}</div>`;
    if (cardSlots.length > 0) {
        bodyHtml = `<div class="cards-grid">${cardSlots.map((c) => c.html).join('')}</div>`;
    } else if (wingetSearching) {
        bodyHtml = `<div class="empty-state">${escapeHtml(t.essSearching)}</div>`;
    } else if (wingetSearchExecuted && unifiedSearchDone) {
        bodyHtml = `<div class="empty-state">${escapeHtml(t.essEmpty)}</div>`;
    }

    const subtitleText = wingetSearching
        ? unifiedSearchPending.length > 0
            ? t.pkgSearchPendingNote(unifiedSearchPending.map((g) => searchPendingGroupLabel(g, unifiedJSMgrs, unifiedPyMgrs)))
            : t.essSearching
        : wingetEnriching
          ? t.essEnrichingSearch
          : t.essSearchSubtitle;

    return `
        <section>
            <div class="section-header">
                <span class="section-title">${escapeHtml(t.essSearchTitle)}</span>
                <span class="section-subtitle">${escapeHtml(subtitleText)}</span>
            </div>
            ${!anyPkgMgrAvailable ? `<div class="conflict-banner" style="margin-bottom:8px">${escapeHtml(t.pkgSearchNoAvailMgrs)}</div>` : ''}
            <div class="search-bar-row">
                <input
                    class="text-input"
                    id="winget-search-input"
                    type="text"
                    value="${escapeHtml(wingetCatalogQuery)}"
                    placeholder="${escapeHtml(t.essSearchPlaceholder)}"
                />
                <button class="btn btn-primary" id="btn-winget-search" ${!anyPkgMgrAvailable ? 'disabled' : ''}>
                    ${escapeHtml(t.essSearchBtn)}
                </button>
                ${
                    wingetSearchExecuted || wingetCatalogQuery.trim()
                        ? `<button class="btn" id="btn-winget-search-clear">${escapeHtml(t.pkgSearchClearBtn)}</button>`
                        : ''
                }
            </div>
            ${filterRowHtml}
            ${pendingBannerHtml}
            ${bodyHtml}
        </section>
    `;
}

function renderAboutModal(s: main.DashboardState): string {
    if (!showAboutModal) return '';
    const t = getT(currentLang());
    const ver = s.appVersion || '1.1.2';

    const linkBtn = (url: string): string =>
        `<button class="detail-link-btn" data-open-url="${escapeHtml(url)}">${escapeHtml(url)}</button>`;

    const infoRows: Array<[string, string, boolean]> = [
        [t.aboutVersionLabel, `<span class="detail-val-mono">v${escapeHtml(ver)}</span>`, false],
        [t.aboutAuthorLabel, escapeHtml('AKZ Team'), true],
        [t.aboutEmailLabel, `<span class="detail-val-mono">${escapeHtml('aozora@botsec.dev')}</span>`, false],
        [t.aboutWebsiteLabel, linkBtn('https://botsec.dev'), false],
        [t.aboutTelegramLabel, linkBtn('https://t.me/AozoraKZ'), false],
        [t.aboutRepoLabel, linkBtn('https://github.com/aozora-kenzu/UniDevUI'), false],
        [t.aboutLicenseLabel, escapeHtml('MIT License (Copyright © 2026 AKZ Team)'), true],
        [t.aboutTechLabel, escapeHtml('Go + Wails v2 + Vanilla TypeScript'), true],
        [t.aboutAppDirLabel, `<span class="detail-val-mono">${escapeHtml(s.appDir)}</span>`, false],
        [t.aboutOutputDirLabel, `<span class="detail-val-mono">${escapeHtml(s.baseDir)}</span>`, false],
    ];

    const gridHtml = infoRows
        .map(([k, val]) => `<div class="detail-key">${escapeHtml(k)}</div><div class="detail-val">${val}</div>`)
        .join('');

    const upd = s.appUpdate;
    const hasUpd = Boolean(upd?.hasUpdate);
    const notesText = (upd?.releaseNotes || '').trim();
    const updStatusLine = checkingAppUpdate
        ? t.aboutUpdateChecking
        : hasUpd
          ? t.aboutUpdateAvailableMsg(upd.currentVersion || ver, upd.latestVersion)
          : t.aboutUpdateUpToDate(ver);

    const updActionsHtml = hasUpd
        ? `
            ${
                upd?.downloadUrl
                    ? `<button class="btn btn-primary btn-sm" id="btn-app-self-update" ${busy || checkingAppUpdate ? 'disabled' : ''}>${escapeHtml(t.aboutUpdateInstallBtn)}${upd.downloadSize > 0 ? ` (${escapeHtml(formatBytes(upd.downloadSize))})` : ''}</button>`
                    : `<span class="about-update-meta">${escapeHtml(t.aboutUpdateNoAssetNote)}</span>`
            }
            ${
                upd?.releaseUrl
                    ? `<button class="btn btn-sm" data-open-url="${escapeHtml(upd.releaseUrl)}">${escapeHtml(t.aboutUpdateOpenReleaseBtn)}</button>`
                    : ''
            }
            <button class="btn btn-sm" id="btn-app-check-update" ${busy || checkingAppUpdate ? 'disabled' : ''}>${escapeHtml(t.aboutUpdateCheckBtn)}</button>
        `
        : `<button class="btn btn-sm" id="btn-app-check-update" ${busy || checkingAppUpdate ? 'disabled' : ''}>${escapeHtml(t.aboutUpdateCheckBtn)}</button>`;

    const updateSectionHtml = `
        <div class="about-update-box ${hasUpd ? 'about-update-warn' : ''}">
            <div class="about-update-row">
                <div>
                    <div class="about-update-heading">${escapeHtml(t.aboutUpdateTitle)}</div>
                    <div class="about-update-meta">${escapeHtml(updStatusLine)}${upd?.checkedAt && !checkingAppUpdate ? ` · ${escapeHtml(t.aboutUpdateLastChecked(upd.checkedAt))}` : ''}</div>
                </div>
                <div class="controls-left">${updActionsHtml}</div>
            </div>
            ${!checkingAppUpdate && upd?.checkError ? `<div class="conflict-banner">${escapeHtml(upd.checkError)}</div>` : ''}
            ${hasUpd && notesText ? `<div class="detail-desc-box about-notes">${escapeHtml(notesText)}</div>` : ''}
        </div>
    `;

    return `
        <div class="modal-backdrop" id="about-modal-backdrop">
            <div class="modal-card detail-modal-card">
                <div class="modal-header">
                    <span class="modal-title">${escapeHtml(t.aboutTitle)}</span>
                    <button class="btn btn-sm" id="btn-about-close">✕</button>
                </div>
                <div class="detail-modal-scroll">
                    <div class="about-hero">
                        <div class="about-brand-mark">&gt;_</div>
                        <div class="about-title-block">
                            <div class="about-app-name">
                                <span>UniDevUI</span>
                                <span class="badge badge-active">v${escapeHtml(ver)}</span>
                            </div>
                            <div class="about-subtitle">${escapeHtml(t.aboutSubtitle)}</div>
                        </div>
                    </div>
                    ${updateSectionHtml}
                    <div class="detail-desc-box">${escapeHtml(t.aboutDesc)}</div>
                    <div class="detail-grid">${gridHtml}</div>
                </div>
                <div class="modal-footer">
                    <div class="controls-left">
                        <button class="btn btn-sm" data-open-url="https://github.com/aozora-kenzu/UniDevUI">${escapeHtml(t.aboutOpenRepo)}</button>
                        <button class="btn btn-sm" data-open-url="https://t.me/AozoraKZ">${escapeHtml(t.aboutOpenTelegram)}</button>
                    </div>
                    <div class="controls-right">
                        <button class="btn btn-primary btn-sm" id="btn-about-close-footer">${escapeHtml(t.essModalClose)}</button>
                    </div>
                </div>
            </div>
        </div>
    `;
}

function getEffectiveSettingsDraft(s: main.DashboardState): config.AppSettings {
    if (settingsDraft) return settingsDraft;
    const base = s.settings || {};
    return config.AppSettings.createFrom({
        baseDir: base.baseDir || s.baseDir || 'C:\\AppPath',
        language: base.language || s.language || 'en',
        closeAction: base.closeAction || 'ask',
        autoStart: Boolean(base.autoStart),
        startInTray: base.startInTray !== undefined ? Boolean(base.startInTray) : true,
        autoCheckUpdates: base.autoCheckUpdates !== undefined ? Boolean(base.autoCheckUpdates) : true,
        autoCheckIntervalHours: base.autoCheckIntervalHours || 6,
        delayStartupScan: base.delayStartupScan !== undefined ? Boolean(base.delayStartupScan) : true,
    });
}

function openSettingsModal(): void {
    if (!state) return;
    settingsDraft = null;
    settingsDraft = getEffectiveSettingsDraft(state);
    showSettingsModal = true;
    render(true);
}

function renderSettingsModal(s: main.DashboardState): string {
    if (!showSettingsModal) return '';
    const t = getT(currentLang());
    const draft = getEffectiveSettingsDraft(s);
    const intervals = [1, 3, 6, 12, 24];

    return `
        <div class="modal-backdrop" id="settings-modal-backdrop">
            <div class="modal-card settings-modal-card">
                <div class="modal-header">
                    <span class="modal-title">${escapeHtml(t.settingsTitle)}</span>
                    <button class="btn btn-sm" id="btn-settings-close">✕</button>
                </div>
                <div class="detail-modal-scroll">
                    <div class="settings-section">
                        <div class="settings-section-title">${escapeHtml(t.settingsSecGeneral)}</div>
                        <div class="settings-field-row">
                            <span class="settings-field-label">${escapeHtml(t.settingsLangLabel)}</span>
                            <select class="select-box" id="set-lang-select">
                                <option value="en" ${draft.language === 'en' ? 'selected' : ''}>English</option>
                                <option value="id" ${draft.language === 'id' ? 'selected' : ''}>Bahasa Indonesia</option>
                            </select>
                        </div>
                        <div class="card-desc">${escapeHtml(t.modalDesc)}</div>
                        <div class="modal-row">
                            <input class="text-input" id="set-basedir-input" type="text" value="${escapeHtml(draft.baseDir)}" placeholder="${escapeHtml(t.modalPlaceholder)}" />
                            <button class="btn" id="btn-set-browse">${escapeHtml(t.modalBrowse)}</button>
                        </div>
                        <div class="controls-left" style="gap:6px">
                            <button class="btn btn-sm" id="btn-set-sync" title="${escapeHtml(t.modalSyncTooltip)}">${escapeHtml(t.modalSyncDefaults)}</button>
                            <button class="btn btn-sm" id="btn-set-reset">${escapeHtml(t.modalReset)}</button>
                        </div>
                        <div class="modal-note">
                            <div><strong>${escapeHtml(t.modalActiveConfig)}</strong> <code>${escapeHtml(s.confPath)}</code> &amp; <code>${escapeHtml(s.essentialsConfPath)}</code></div>
                            <div>${escapeHtml(t.modalCopyNote)}</div>
                        </div>
                    </div>

                    <div class="settings-section">
                        <div class="settings-section-title">${escapeHtml(t.settingsSecWindow)}</div>
                        <div class="settings-field-row">
                            <span class="settings-field-label">${escapeHtml(t.settingsCloseActionLabel)}</span>
                            <select class="select-box" id="set-close-action">
                                <option value="ask" ${draft.closeAction === 'ask' ? 'selected' : ''}>${escapeHtml(t.settingsCloseAsk)}</option>
                                <option value="tray" ${draft.closeAction === 'tray' ? 'selected' : ''}>${escapeHtml(t.settingsCloseTray)}</option>
                                <option value="quit" ${draft.closeAction === 'quit' ? 'selected' : ''}>${escapeHtml(t.settingsCloseQuit)}</option>
                            </select>
                        </div>
                        <label class="settings-toggle-row">
                            <input type="checkbox" id="chk-set-autostart" ${draft.autoStart ? 'checked' : ''} />
                            <div>
                                <div class="settings-toggle-title">${escapeHtml(t.settingsAutoStartLabel)}</div>
                                <div class="settings-toggle-desc">${escapeHtml(t.settingsAutoStartDesc)}</div>
                            </div>
                        </label>
                        <label class="settings-toggle-row settings-sub-row ${!draft.autoStart ? 'disabled' : ''}">
                            <input type="checkbox" id="chk-set-start-tray" ${draft.startInTray ? 'checked' : ''} ${!draft.autoStart ? 'disabled' : ''} />
                            <div>
                                <div class="settings-toggle-title">${escapeHtml(t.settingsStartInTrayLabel)}</div>
                                <div class="settings-toggle-desc">${escapeHtml(t.settingsStartInTrayDesc)}</div>
                            </div>
                        </label>
                    </div>

                    <div class="settings-section">
                        <div class="settings-section-title">${escapeHtml(t.settingsSecUpdates)}</div>
                        <label class="settings-toggle-row">
                            <input type="checkbox" id="chk-set-autocheck" ${draft.autoCheckUpdates ? 'checked' : ''} />
                            <div>
                                <div class="settings-toggle-title">${escapeHtml(t.settingsAutoCheckLabel)}</div>
                                <div class="settings-toggle-desc">${escapeHtml(t.settingsAutoCheckDesc)}</div>
                            </div>
                        </label>
                        <div class="settings-field-row settings-sub-row">
                            <span class="settings-field-label">${escapeHtml(t.settingsIntervalLabel)}</span>
                            <select class="select-box" id="set-check-interval" ${!draft.autoCheckUpdates ? 'disabled' : ''}>
                                ${intervals
                                    .map(
                                        (h) =>
                                            `<option value="${h}" ${draft.autoCheckIntervalHours === h ? 'selected' : ''}>${escapeHtml(t.settingsIntervalHours(h))}</option>`,
                                    )
                                    .join('')}
                            </select>
                        </div>
                        <label class="settings-toggle-row">
                            <input type="checkbox" id="chk-set-delay-scan" ${draft.delayStartupScan ? 'checked' : ''} />
                            <div>
                                <div class="settings-toggle-title">${escapeHtml(t.settingsDelayScanLabel)}</div>
                                <div class="settings-toggle-desc">${escapeHtml(t.settingsDelayScanDesc)}</div>
                            </div>
                        </label>
                    </div>
                </div>
                <div class="modal-footer">
                    <div></div>
                    <div class="controls-right">
                        <button class="btn btn-sm" id="btn-settings-cancel">${escapeHtml(t.modalCancel)}</button>
                        <button class="btn btn-primary btn-sm" id="btn-settings-save">${escapeHtml(t.settingsSaveBtn)}</button>
                    </div>
                </div>
            </div>
        </div>
    `;
}

function renderCloseConfirmModal(): string {
    if (!showCloseConfirmModal) return '';
    const t = getT(currentLang());
    return `
        <div class="modal-backdrop" id="close-confirm-backdrop">
            <div class="modal-card close-modal-card">
                <div class="modal-header">
                    <span class="modal-title">${escapeHtml(t.closeConfirmTitle)}</span>
                    <button class="btn btn-sm" id="btn-close-confirm-x">✕</button>
                </div>
                <div class="close-modal-hero">
                    <div class="close-modal-icon">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 21h8M12 18v3M8 11l4 4 4-4"/></svg>
                    </div>
                    <div class="card-desc" style="margin:0">${escapeHtml(t.closeConfirmDesc)}</div>
                </div>
                <label class="settings-toggle-row">
                    <input type="checkbox" id="chk-close-remember" ${closeConfirmRemember ? 'checked' : ''} />
                    <span class="settings-toggle-title">${escapeHtml(t.closeConfirmRemember)}</span>
                </label>
                <div class="modal-footer">
                    <button class="btn btn-sm" id="btn-close-cancel">${escapeHtml(t.modalCancel)}</button>
                    <div class="controls-right">
                        <button class="btn btn-danger btn-sm" id="btn-close-quit">${escapeHtml(t.closeConfirmQuitBtn)}</button>
                        <button class="btn btn-primary btn-sm" id="btn-close-tray">${escapeHtml(t.closeConfirmTrayBtn)}</button>
                    </div>
                </div>
            </div>
        </div>
    `;
}

function renderDetailModal(): string {
    if (!detailTarget) return '';
    const t = getT(currentLang());
    const mgr = (detailTarget.manager || 'winget').toLowerCase();
    const det = getCachedDetails(detailTarget.wingetId, detailTarget.source, mgr);
    const curated = mgr === 'winget' && detailTarget.appId ? getCuratedMeta(detailTarget.appId) : undefined;
    const pinnedIDs = new Set((state?.essentials || []).map((e) => e.wingetId.toLowerCase()));
    const isPinned = mgr === 'winget' && pinnedIDs.has(detailTarget.wingetId.toLowerCase());
    const mgrReady = isExtraManagerAvailable(mgr);

    const title = det?.name || detailTarget.name;
    const publisher = det?.publisher || curated?.publisher || det?.author || '—';
    const homepage = det?.homepage || det?.publisherUrl || '';
    const version = det?.version || detailTarget.version || '—';
    const license = det?.license || curated?.license || (det?.licenseUrl ? shortDomain(det.licenseUrl) || 'License' : '—');
    const description = det?.description || detailTarget.fallbackDesc || '';
    const iconHtml = renderAppIcon(detailTarget.appId || detailTarget.wingetId, title, {
        wingetId: mgr === 'winget' ? detailTarget.wingetId : undefined,
        dataUrl: det?.iconDataUrl,
        iconUrl: det?.iconUrl || detailTarget.iconUrl,
        sizeClass: 'lg',
    });

    const rows: string[] = [
        `<div class="detail-key">${escapeHtml(t.essPublisherLabel)}</div><div class="detail-val">${escapeHtml(publisher)}</div>`,
    ];
    if (det?.author && det.author !== publisher) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essAuthorLabel)}</div><div class="detail-val">${escapeHtml(det.author)}</div>`);
    }
    rows.push(`<div class="detail-key">${escapeHtml(t.essVersionLabel.replace(/:$/, ''))}</div><div class="detail-val detail-val-mono">${escapeHtml(version)}</div>`);
    rows.push(
        det?.licenseUrl
            ? `<div class="detail-key">${escapeHtml(t.essLicenseLabel)}</div><div class="detail-val"><button class="detail-link-btn" data-open-url="${escapeHtml(det.licenseUrl)}">${escapeHtml(license)}</button></div>`
            : `<div class="detail-key">${escapeHtml(t.essLicenseLabel)}</div><div class="detail-val">${escapeHtml(license)}</div>`,
    );
    if (det?.downloadCount && det.downloadCount > 0) {
        rows.push(`<div class="detail-key">${escapeHtml(t.pkgDownloadCountLabel)}</div><div class="detail-val detail-val-mono">${escapeHtml(formatDownloadCount(det.downloadCount))}</div>`);
    }
    const modalEssSize =
        det?.installerSize ||
        (mgr === 'winget'
            ? (state?.essentials || []).find((e) => e.wingetId.toLowerCase() === detailTarget!.wingetId.toLowerCase())?.downloadSize || 0
            : 0);
    if (det?.releaseDate) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essReleaseDateLabel)}</div><div class="detail-val detail-val-mono">${escapeHtml(det.releaseDate)}</div>`);
    }
    if (modalEssSize > 0) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essInstallerSizeLabel)}</div><div class="detail-val detail-val-mono">${escapeHtml(formatBytes(modalEssSize))}</div>`);
    }
    if (det?.installerType) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essInstallerTypeLabel)}</div><div class="detail-val detail-val-mono">${escapeHtml(det.installerType)}</div>`);
    }
    if (homepage) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essHomepageLabel)}</div><div class="detail-val"><button class="detail-link-btn" data-open-url="${escapeHtml(homepage)}">${escapeHtml(homepage)}</button></div>`);
    }
    if (det?.installerUrl) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essInstallerUrlLabel)}</div><div class="detail-val detail-val-mono">${escapeHtml(det.installerUrl)}</div>`);
    }
    if (det?.installerSha256) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essSha256Label)}</div><div class="detail-val detail-val-mono">${escapeHtml(det.installerSha256)}</div>`);
    }

    const screenshotsHtml =
        det?.screenshots && det.screenshots.length > 0
            ? `
            <div>
                <div class="nav-section-title" style="padding:0 0 4px">${escapeHtml(t.pkgScreenshotsLabel)}</div>
                <div class="detail-screenshots-strip">
                    ${det.screenshots
                        .map(
                            (shot) => `
                            <button class="detail-shot-card" data-open-url="${escapeHtml(shot)}" title="${escapeHtml(shot)}">
                                <img class="detail-shot-img" src="${escapeHtml(shot)}" loading="lazy" alt="" />
                            </button>
                        `,
                        )
                        .join('')}
                </div>
            </div>
        `
            : '';

    const depsHtml =
        det?.dependencies && det.dependencies.length > 0
            ? `
            <div>
                <div class="nav-section-title" style="padding:0 0 4px">${escapeHtml(t.pkgDependenciesLabel)}</div>
                <div class="detail-tags">
                    ${det.dependencies.map((dp) => `<span class="badge badge-idle">${escapeHtml(dp)}</span>`).join('')}
                </div>
            </div>
        `
            : '';

    return `
        <div class="modal-backdrop" id="detail-modal-backdrop">
            <div class="modal-card detail-modal-card">
                <div class="modal-header">
                    <div class="detail-header-row">
                        ${iconHtml}
                        <div>
                            <div class="modal-title">${escapeHtml(title)}</div>
                            <div class="card-title-row" style="margin-top:3px">
                                <span class="card-id">[${escapeHtml(detailTarget.wingetId)}]</span>
                                ${renderManagerBadge(mgr, detailTarget.source)}
                            </div>
                        </div>
                    </div>
                    <button class="btn btn-sm" id="btn-detail-close">✕</button>
                </div>

                <div class="detail-modal-scroll">
                    ${detailLoading ? `<div class="modal-note">${escapeHtml(t.essDetailsLoading)}</div>` : ''}
                    ${detailError ? `<div class="conflict-banner">${escapeHtml(detailError)}</div>` : ''}
                    <div class="detail-grid">${rows.join('')}</div>
                    ${screenshotsHtml}
                    ${
                        description
                            ? `
                            <div>
                                <div class="nav-section-title" style="padding:0 0 4px">${escapeHtml(t.essDescriptionLabel)}</div>
                                <div class="detail-desc-box">${escapeHtml(description)}</div>
                            </div>
                        `
                            : !detailLoading
                              ? `<div class="modal-note">${escapeHtml(t.essDetailsNoData)}</div>`
                              : ''
                    }
                    ${depsHtml}
                    ${
                        det?.tags && det.tags.length > 0
                            ? `
                            <div>
                                <div class="nav-section-title" style="padding:0 0 4px">${escapeHtml(t.essTagsLabel)}</div>
                                <div class="detail-tags">
                                    ${det.tags.map((tg) => `<span class="badge badge-idle">${escapeHtml(tg)}</span>`).join('')}
                                </div>
                            </div>
                        `
                            : ''
                    }
                </div>

                <div class="modal-footer">
                    <div class="controls-left">
                        <button
                            class="btn btn-primary btn-sm"
                            id="btn-detail-install"
                            ${busy || !mgrReady ? 'disabled' : ''}
                        >
                            ${escapeHtml(t.essInstallSilent)}
                        </button>
                        ${
                            mgr === 'winget' && !isPinned
                                ? `<button class="btn btn-sm" id="btn-detail-pin" ${busy ? 'disabled' : ''}>${escapeHtml(t.essPinToList)}</button>`
                                : ''
                        }
                        ${
                            homepage
                                ? `<button class="btn btn-sm" data-open-url="${escapeHtml(homepage)}">${escapeHtml(t.essOpenHomepage)}</button>`
                                : ''
                        }
                    </div>
                    <div class="controls-right">
                        <button class="btn btn-sm" id="btn-detail-close-footer">${escapeHtml(t.essModalClose)}</button>
                    </div>
                </div>
            </div>
        </div>
    `;
}

function openPackageDetailModal(target: DetailTarget): void {
    detailTarget = target;
    detailError = '';
    const mgr = (target.manager || 'winget').toLowerCase();
    const existing = getCachedDetails(target.wingetId, target.source, mgr);
    if (existing && existing.loaded) {
        detailLoading = false;
        render(true);
        return;
    }

    detailLoading = true;
    render(true);

    GetUniversalPackageDetails(mgr, target.wingetId, target.source)
        .then((det) => {
            wingetDetailsByKey[detailKey(target.wingetId, target.source, mgr)] = det;
            if (det.id) {
                wingetDetailsByKey[detailKey(det.id, det.source || target.source, mgr)] = det;
            }
            detailLoading = false;
            render(true);
        })
        .catch((e) => {
            detailLoading = false;
            detailError = e instanceof Error ? e.message : String(e);
            render(true);
        });
}

function hydrateDetailsBatch(
    entries: Array<{ name: string; wingetId: string; version?: string; source?: string }>,
    limit = 8,
): void {
    const missing = entries.filter((e) => e.wingetId && !customEssHydrated[detailKey(e.wingetId, e.source)]);
    if (missing.length === 0) return;

    const batch = missing.slice(0, limit);
    for (const item of batch) {
        customEssHydrated[detailKey(item.wingetId, item.source)] = true;
    }

    const rows = batch.map((e) =>
        essentials.WingetTableRow.createFrom({
            name: e.name,
            id: e.wingetId,
            version: e.version || '',
            source: e.source || 'winget',
        }),
    );

    EnrichWingetSearch(rows)
        .then((detailsList) => {
            let updated = false;
            for (const det of detailsList || []) {
                if (det && det.id) {
                    wingetDetailsByKey[detailKey(det.id, det.source)] = det;
                    updated = true;
                }
            }
            if (updated && primaryTab === 'essentials') {
                render(true);
            }
        })
        .catch(() => {
            // Non-fatal background enrichment.
        });
}

function hydrateCustomEssentialsIfNeeded(list: main.EssentialView[]): void {
    const candidates = list
        .filter((e) => !getCuratedMeta(e.id))
        .map((e) => ({
            name: e.name,
            wingetId: e.wingetId,
            version: e.installedVer || '',
            source: e.source || 'winget',
        }));
    hydrateDetailsBatch(candidates, 8);
}

function hydrateInstalledAppsIfNeeded(list: main.InstalledAppView[]): void {
    const manageable = list.filter(
        (a) => (a.manager || 'winget') === 'winget' && a.managedByWinget && (a.hasUpdate || installedFilter === 'all'),
    );
    // Prioritize updatable entries first so download sizes & icons appear right away.
    const sorted = [...manageable].sort((a, b) => Number(b.hasUpdate) - Number(a.hasUpdate));
    hydrateDetailsBatch(
        sorted.map((a) => ({
            name: a.name,
            wingetId: a.id,
            version: a.installedVer || '',
            source: a.source || 'winget',
        })),
        10,
    );
}

function renderInstalledAppCard(app: main.InstalledAppView): string {
    const t = getT(currentLang());
    const mgr = (app.manager || 'winget').toLowerCase();
    const isWinget = mgr === 'winget';
    const det = isWinget ? getCachedDetails(app.id, app.source) : undefined;
    const curated = isWinget && app.curatedAppId ? getCuratedMeta(app.curatedAppId) : undefined;
    const iconID = app.curatedAppId || app.id;
    const mgrReady = isExtraManagerAvailable(mgr);
    const dlSize = app.downloadSize || det?.installerSize || 0;
    const desc = det?.description || app.description || '';

    const metaStrip = renderMetaStrip({
        publisher: det?.publisher || curated?.publisher,
        domain: shortDomain(det?.homepage || det?.publisherUrl || app.homepage) || curated?.domain,
        license: det?.license || curated?.license,
        installerType: det?.installerType,
    });

    const sourceBadge = isWinget
        ? app.managedByWinget
            ? `<span class="badge ${app.source === 'msstore' ? 'badge-store' : 'badge-cat'}">${escapeHtml(app.source || 'winget')}</span>`
            : `<span class="badge badge-idle" title="${escapeHtml(t.essLocalEntryNoUpdateTip)}">${escapeHtml(t.essBadgeLocalEntry)}</span>`
        : renderManagerBadge(mgr, app.source);

    const statusBadge = app.ignored
        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeIgnored)}</span>`
        : app.pauseUntil
          ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgePausedUntil(app.pauseUntil))}</span>`
          : app.skippedVersion
            ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgeSkipped(app.skippedVersion))}</span>`
            : app.hasUpdate
              ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(app.availableVer))}</span>`
              : `<span class="badge badge-active">${escapeHtml(app.installedVer ? t.essBadgeInstalled(app.installedVer) : t.statusDetected)}</span>`;

    const compareBadge = app.ignored
        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeUpdateIgnored)}</span>`
        : app.pauseUntil
          ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgePausedUntil(app.pauseUntil))}</span>`
          : app.skippedVersion
            ? `<span class="badge badge-ignored">${escapeHtml(t.pkgBadgeSkipped(app.skippedVersion))}</span>`
            : app.hasUpdate
              ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(app.availableVer))}</span>`
              : `<span class="badge badge-active">${escapeHtml(t.statusUpToDate)}</span>`;

    const actionsHtml = app.ignored
        ? `
            <button
                class="btn btn-sm btn-restore"
                data-action="winget-unignore"
                data-mgr="${escapeHtml(mgr)}"
                data-wid="${escapeHtml(app.id)}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnRestoreTip)}"
            >
                ${escapeHtml(t.essBtnRestore)}
            </button>
        `
        : `
            ${
                app.hasUpdate && app.canUpdate
                    ? `
                    <button
                        class="btn btn-primary btn-sm"
                        data-action="pkg-action"
                        data-mgr="${escapeHtml(mgr)}"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || '')}"
                        data-mode="update"
                        ${busy || !mgrReady ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.btnUpdateTool)}
                    </button>
                    <button
                        class="btn btn-sm"
                        data-action="winget-skip-ver"
                        data-mgr="${escapeHtml(mgr)}"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || '')}"
                        data-ver="${escapeHtml(app.availableVer)}"
                        ${busy ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnSkipVersionTip(app.availableVer))}"
                    >
                        ${escapeHtml(`${t.pkgBtnSkipVersion} ${app.availableVer}`)}
                    </button>
                    <button
                        class="btn btn-sm"
                        data-action="winget-pause-7d"
                        data-mgr="${escapeHtml(mgr)}"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || '')}"
                        data-days="7"
                        ${busy ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnPause7dTip)}"
                    >
                        ${escapeHtml(t.pkgBtnPause7d)}
                    </button>
                `
                    : ''
            }
            ${
                app.pauseUntil
                    ? `
                    <button
                        class="btn btn-sm btn-restore"
                        data-action="winget-unignore"
                        data-mgr="${escapeHtml(mgr)}"
                        data-wid="${escapeHtml(app.id)}"
                        ${busy ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnResumeUpdatesTip)}"
                    >
                        ${escapeHtml(t.pkgBtnResumeUpdates)}
                    </button>
                `
                    : app.skippedVersion
                      ? `
                    <button
                        class="btn btn-sm btn-restore"
                        data-action="winget-unignore"
                        data-mgr="${escapeHtml(mgr)}"
                        data-wid="${escapeHtml(app.id)}"
                        ${busy ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnClearSkipTip)}"
                    >
                        ${escapeHtml(t.pkgBtnClearSkip)}
                    </button>
                `
                      : ''
            }
            <button
                class="btn btn-sm"
                data-action="winget-ignore"
                data-mgr="${escapeHtml(mgr)}"
                data-name="${escapeHtml(app.name)}"
                data-wid="${escapeHtml(app.id)}"
                data-source="${escapeHtml(app.source || '')}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnIgnoreTip)}"
            >
                ${escapeHtml(t.essBtnIgnore)}
            </button>
            ${
                isWinget && app.managedByWinget && !app.pinnedCurated
                    ? `
                    <button
                        class="btn btn-sm"
                        data-action="winget-pin"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || 'winget')}"
                        ${busy ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.essPinToList)}
                    </button>
                `
                    : isWinget && app.pinnedCurated
                      ? `<span class="badge badge-cat">${escapeHtml(t.essAlreadyPinned)}</span>`
                      : ''
            }
            ${
                !isWinget && app.homepage
                    ? `<button class="btn btn-sm" data-open-url="${escapeHtml(app.homepage)}">${escapeHtml(t.essOpenHomepage)}</button>`
                    : ''
            }
            ${
                app.canUninstall
                    ? `
                    <button
                        class="btn btn-danger btn-sm"
                        data-action="pkg-action"
                        data-mgr="${escapeHtml(mgr)}"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || '')}"
                        data-mode="uninstall"
                        ${busy || !mgrReady ? 'disabled' : ''}
                        title="${escapeHtml(t.pkgBtnUninstallTip)}"
                    >
                        ${escapeHtml(t.pkgBtnUninstall)}
                    </button>
                `
                    : ''
            }
        `;

    return `
        <div class="card ${!app.ignored ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(iconID, app.name, { wingetId: isWinget ? app.id : undefined, dataUrl: det?.iconDataUrl, iconUrl: app.iconUrl || det?.iconUrl })}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(det?.name || app.name)}</span>
                            <span class="card-id">[${escapeHtml(app.id)}]</span>
                            ${sourceBadge}
                            ${app.ignored && app.wingetPinned ? `<span class="badge badge-cat">${escapeHtml(t.essBadgeWingetPinned)}</span>` : ''}
                        </div>
                        ${desc ? `<div class="card-desc card-desc-clamp">${escapeHtml(desc)}</div>` : ''}
                        ${app.ignoreReason && app.ignored ? `<div class="card-desc">${escapeHtml(app.ignoreReason)}</div>` : ''}
                        ${metaStrip}
                    </div>
                </div>
                ${statusBadge}
            </div>

            <div class="version-compare">
                <div class="vc-group">
                    <span class="vc-item">
                        <span class="vc-label">${escapeHtml(t.installedLabel)}</span>
                        <span class="vc-val">${escapeHtml(app.installedVer ? `v${app.installedVer}` : t.statusDetected)}</span>
                    </span>
                    ${
                        app.availableVer
                            ? `
                            <span class="vc-sep">·</span>
                            <span class="vc-item">
                                <span class="vc-label">${escapeHtml(isWinget ? t.essWingetLatestLabel : t.latestLabel)}</span>
                                <span class="vc-val">v${escapeHtml(app.availableVer)}</span>
                            </span>
                        `
                            : ''
                    }
                </div>
                ${compareBadge}
            </div>

            <div class="card-controls">
                <div class="controls-left">
                    ${actionsHtml}
                    ${
                        !isWinget || app.managedByWinget
                            ? `
                            <button
                                class="btn btn-sm"
                                data-action="ess-detail"
                                data-mgr="${escapeHtml(mgr)}"
                                data-ess="${escapeHtml(app.curatedAppId || '')}"
                                data-name="${escapeHtml(det?.name || app.name)}"
                                data-wid="${escapeHtml(app.id)}"
                                data-source="${escapeHtml(app.source || mgr)}"
                                data-ver="${escapeHtml(app.availableVer || app.installedVer || '')}"
                                data-desc="${escapeHtml(det?.description || app.description || '')}"
                                data-iconurl="${escapeHtml(app.iconUrl || det?.iconUrl || '')}"
                                data-isess="${app.pinnedCurated ? '1' : '0'}"
                            >
                                ${escapeHtml(t.essDetailsBtn)}
                            </button>
                        `
                            : ''
                    }
                    ${
                        dlSize > 0
                            ? `<span class="size-pill" title="${escapeHtml(t.downloadSizeLabel(formatBytes(dlSize)))}">↓ ${escapeHtml(formatBytes(dlSize))}</span>`
                            : ''
                    }
                </div>
            </div>
        </div>
    `;
}

function renderInstalledWorkspaceView(s: main.DashboardState): string {
    const t = getT(currentLang());
    const allRows = s.installedApps || [];
    const activeUpdates = allRows.filter((a) => !a.ignored && a.hasUpdate);
    const activeInstalled = allRows.filter((a) => !a.ignored);
    const ignoredList = allRows.filter((a) => a.ignored || Boolean(a.skippedVersion) || Boolean(a.pauseUntil));

    const statusList =
        installedFilter === 'updates' ? activeUpdates : installedFilter === 'ignored' ? ignoredList : activeInstalled;

    const mgrOrder: ManagerKey[] = ['winget', 'scoop', 'choco', 'npm', 'pnpm', 'bun', 'uv', 'pip', 'cargo', 'dotnet', 'psgallery'];
    const mgrCounts: Record<string, number> = {};
    for (const row of statusList) {
        const m = (row.manager || 'winget').toLowerCase();
        mgrCounts[m] = (mgrCounts[m] || 0) + 1;
    }

    const mgrChips = mgrOrder
        .filter((m) => (mgrCounts[m] || 0) > 0 || installedMgrFilter === m)
        .map(
            (m) => `
            <button class="mgr-chip ${installedMgrFilter === m ? 'active' : ''}" data-installed-mgr="${m}">
                <span>${escapeHtml(managerDisplayName(m))}</span>
                <span class="mgr-chip-count">${mgrCounts[m] || 0}</span>
            </button>
        `,
        )
        .join('');

    const filtered = statusList.filter((a) => {
        const m = (a.manager || 'winget').toLowerCase();
        if (installedMgrFilter !== 'all' && m !== installedMgrFilter) return false;
        return matchesSearch(`${a.name} ${a.id} ${a.installedVer} ${a.availableVer} ${a.source} ${m}`, essentialSearchQuery);
    });

    const updatableCount = activeUpdates.filter(
        (a) => a.canUpdate && (installedMgrFilter === 'all' || (a.manager || 'winget').toLowerCase() === installedMgrFilter),
    ).length;
    const emptyMsg =
        installedFilter === 'updates'
            ? t.essUpdatesEmpty
            : installedFilter === 'ignored'
              ? t.essIgnoredEmpty
              : t.essInstalledEmpty;

    return `
        <section>
            <div class="section-header">
                <span class="section-title">${escapeHtml(t.essInstalledTitle)} (${filtered.length})</span>
                <span class="section-subtitle">${escapeHtml(t.essInstalledSubtitle)}</span>
            </div>

            <div class="subfilter-bar">
                <div class="subfilter-tabs">
                    <button class="subfilter-tab ${installedFilter === 'updates' ? 'active' : ''}" data-installed-filter="updates">
                        ${escapeHtml(t.essFilterUpdates(activeUpdates.length))}
                    </button>
                    <button class="subfilter-tab ${installedFilter === 'all' ? 'active' : ''}" data-installed-filter="all">
                        ${escapeHtml(t.essFilterAllInstalled(activeInstalled.length))}
                    </button>
                    <button class="subfilter-tab ${installedFilter === 'ignored' ? 'active' : ''}" data-installed-filter="ignored">
                        ${escapeHtml(t.essFilterIgnored(ignoredList.length))}
                    </button>
                </div>

                <div class="controls-left">
                    <button class="btn btn-sm" id="btn-ignored-conf" title="${escapeHtml(t.btnIgnoredConfTooltip)}">
                        ${escapeHtml(t.btnIgnoredConf)}
                    </button>
                    <button
                        class="btn btn-primary btn-sm"
                        id="btn-update-all-installed"
                        ${busy || updatableCount === 0 ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.essUpdateAll(updatableCount))}
                    </button>
                </div>
            </div>

            ${
                mgrChips
                    ? `
                    <div class="mgr-filter-row">
                        <button class="mgr-chip ${installedMgrFilter === 'all' ? 'active' : ''}" data-installed-mgr="all">
                            ${escapeHtml(t.pkgFilterAllManagers(statusList.length))}
                        </button>
                        ${mgrChips}
                    </div>
                `
                    : ''
            }

            <div class="cards-grid">
                ${
                    filtered.length > 0
                        ? filtered.map(renderInstalledAppCard).join('')
                        : `<div class="empty-state">${escapeHtml(emptyMsg)}</div>`
                }
            </div>
        </section>
    `;
}

function renderPathInspector(s: main.DashboardState): string {
    const t = getT(currentLang());
    const allEntries =
        s.userPathEntries && s.userPathEntries.length > 0
            ? s.userPathEntries
            : (s.userPathList || []).map((p, idx) => ({
                  index: idx,
                  path: p,
                  expanded: p,
                  exists: true,
                  managed: p.toLowerCase().startsWith(s.baseDir.toLowerCase()),
                  duplicate: false,
              }));

    const isFiltered = runtimeSearchQuery.trim().length > 0;
    const filtered = allEntries.filter((e) => matchesSearch(`${e.path} ${e.expanded}`, runtimeSearchQuery));
    const issueCount = allEntries.filter((e) => !e.exists || e.duplicate).length;

    const rows = filtered
        .map((item) => {
            const titleHint = item.expanded && item.expanded !== item.path ? `title="${escapeHtml(item.expanded)}"` : '';
            const rowClass = `${item.managed ? 'managed' : ''} ${!item.exists ? 'missing' : ''}`.trim();
            return `
                <div class="path-entry-row ${rowClass}">
                    <span class="path-index">#${item.index + 1}</span>
                    <div class="path-entry-main">
                        <span class="path-entry-text" ${titleHint}>${escapeHtml(item.path)}</span>
                        ${item.managed ? `<span class="badge badge-active">${escapeHtml(t.managedBadge)}</span>` : ''}
                        ${!item.exists ? `<span class="badge badge-warn">${escapeHtml(t.pathBadgeMissing)}</span>` : ''}
                        ${item.duplicate ? `<span class="badge badge-idle">${escapeHtml(t.pathBadgeDuplicate)}</span>` : ''}
                    </div>
                    <div class="path-entry-actions">
                        <button
                            class="btn btn-xs"
                            data-action="path-move"
                            data-idx="${item.index}"
                            data-delta="-1"
                            ${busy || isFiltered || item.index === 0 ? 'disabled' : ''}
                            title="${escapeHtml(t.pathBtnMoveUpTip)}"
                        >↑</button>
                        <button
                            class="btn btn-xs"
                            data-action="path-move"
                            data-idx="${item.index}"
                            data-delta="1"
                            ${busy || isFiltered || item.index === allEntries.length - 1 ? 'disabled' : ''}
                            title="${escapeHtml(t.pathBtnMoveDownTip)}"
                        >↓</button>
                        <button
                            class="btn btn-xs"
                            data-action="path-open"
                            data-dir="${escapeHtml(item.path)}"
                            ${!item.exists ? 'disabled' : ''}
                            title="${escapeHtml(t.pathBtnOpenTip)}"
                        >${escapeHtml(t.pathBtnOpen)}</button>
                        <button
                            class="btn btn-danger btn-xs"
                            data-action="path-remove"
                            data-idx="${item.index}"
                            data-dir="${escapeHtml(item.path)}"
                            ${busy ? 'disabled' : ''}
                            title="${escapeHtml(t.pathBtnRemoveTip)}"
                        >${escapeHtml(t.pathBtnRemove)}</button>
                    </div>
                </div>
            `;
        })
        .join('');

    return `
        <div>
            <div class="section-header">
                <span class="section-title">${escapeHtml(t.pathInspectorTitle)}</span>
                <span class="section-subtitle">${escapeHtml(t.pathInspectorSubtitle(allEntries.length))}</span>
            </div>
            <div class="path-toolbar">
                <div class="controls-left">
                    <button class="btn btn-primary btn-sm" id="btn-path-add" ${busy ? 'disabled' : ''}>
                        ${escapeHtml(t.pathBtnAddFolder)}
                    </button>
                    <button class="btn btn-sm" id="btn-path-clean" ${busy || issueCount === 0 ? 'disabled' : ''}>
                        ${escapeHtml(t.pathBtnClean(issueCount))}
                    </button>
                </div>
                <button class="btn btn-sm" id="btn-path-refresh" ${busy ? 'disabled' : ''}>
                    ${escapeHtml(t.pathBtnRefresh)}
                </button>
            </div>
            <div class="path-table">
                ${rows || `<div class="empty-state">${escapeHtml(t.pathInspectorEmpty)}</div>`}
            </div>
        </div>
    `;
}

function renderTerminalLinesOnly(): void {
    const bodyEl = document.getElementById('terminal-body');
    if (!bodyEl) return;

    const lang = currentLang();
    bodyEl.innerHTML = logs
        .map((l) => {
            const msg = lang === 'id' && l.messageId ? l.messageId : l.message;
            return `
            <div class="log-line">
                <span class="log-ts">${escapeHtml(l.timestamp)}</span>
                <span class="log-lvl lvl-${escapeHtml(l.level)}">[${escapeHtml(l.level)}]</span>
                <span>${escapeHtml(msg)}</span>
            </div>
        `;
        })
        .join('');
    bodyEl.scrollTop = bodyEl.scrollHeight;
}

function renderRuntimesView(s: main.DashboardState): { sidebarHtml: string; contentHtml: string } {
    const t = getT(currentLang());
    const corePkgs = s.packages.filter(
        (p) => p.category !== 'stack' && matchesSearch(`${p.name} ${p.id} ${p.description}`, runtimeSearchQuery),
    );
    const stackPkgs = s.packages.filter(
        (p) => p.category === 'stack' && matchesSearch(`${p.name} ${p.id} ${p.description}`, runtimeSearchQuery),
    );
    const matchesTool = (tl: tools.ToolStatus): boolean =>
        matchesSearch(`${tl.name} ${tl.id} ${tl.description} ${tl.commandHint}`, runtimeSearchQuery);
    const allCliSdk = s.tools.filter((tl) => tl.category !== 'pkgmgr');
    const allPkgMgr = s.tools.filter((tl) => tl.category === 'pkgmgr');
    const cliTools = allCliSdk.filter(matchesTool);
    const pkgMgrTools = allPkgMgr.filter(matchesTool);
    const totalCore = s.packages.filter((p) => p.category !== 'stack').length;
    const totalStack = s.packages.filter((p) => p.category === 'stack').length;

    let contentHtml = '';
    if (runtimeTab === 'path') {
        contentHtml = renderPathInspector(s);
    } else {
        if (runtimeTab === 'all' || runtimeTab === 'core') {
            contentHtml += `
                <section>
                    <div class="section-header">
                        <span class="section-title">${escapeHtml(t.secCoreTitle)}</span>
                        <span class="section-subtitle">${escapeHtml(t.secCoreSubtitle(s.baseDir))}</span>
                    </div>
                    <div class="cards-grid">
                        ${corePkgs.length > 0 ? corePkgs.map(renderPackageCard).join('') : `<div class="empty-state">${escapeHtml(t.emptyCore)}</div>`}
                    </div>
                </section>
            `;
        }
        if (runtimeTab === 'all' || runtimeTab === 'stack') {
            contentHtml += `
                <section>
                    <div class="section-header">
                        <span class="section-title">${escapeHtml(t.secStackTitle)}</span>
                        <span class="section-subtitle">${escapeHtml(t.secStackSubtitle)}</span>
                    </div>
                    <div class="cards-grid">
                        ${stackPkgs.length > 0 ? stackPkgs.map(renderPackageCard).join('') : `<div class="empty-state">${escapeHtml(t.emptyStack)}</div>`}
                    </div>
                </section>
            `;
        }
        if (runtimeTab === 'all' || runtimeTab === 'tools') {
            contentHtml += `
                <section>
                    <div class="section-header">
                        <span class="section-title">${escapeHtml(t.secToolsTitle)}</span>
                        <span class="section-subtitle">${escapeHtml(t.secToolsSubtitle)}</span>
                    </div>
                    <div class="cards-grid">
                        ${cliTools.length > 0 ? cliTools.map(renderToolCard).join('') : `<div class="empty-state">${escapeHtml(t.emptyTools)}</div>`}
                    </div>
                </section>
            `;
        }
        if (runtimeTab === 'all' || runtimeTab === 'pkgmgr') {
            contentHtml += `
                <section>
                    <div class="section-header">
                        <span class="section-title">${escapeHtml(t.secPkgMgrTitle)}</span>
                        <span class="section-subtitle">${escapeHtml(t.secPkgMgrSubtitle)}</span>
                    </div>
                    <div class="cards-grid">
                        ${pkgMgrTools.length > 0 ? pkgMgrTools.map(renderToolCard).join('') : `<div class="empty-state">${escapeHtml(t.emptyPkgMgr)}</div>`}
                    </div>
                </section>
            `;
        }
    }

    const sidebarHtml = `
        <input class="search-input" id="search-input" type="text" placeholder="${escapeHtml(t.searchPlaceholder)}" value="${escapeHtml(runtimeSearchQuery)}" />
        <div class="nav-section-title">${escapeHtml(t.navWorkspace)}</div>
        <div class="nav-list">
            <button class="nav-item ${runtimeTab === 'all' ? 'active' : ''}" data-rt-tab="all">
                <span>${escapeHtml(t.navAll)}</span>
                <span class="nav-count">${s.packages.length + s.tools.length}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'core' ? 'active' : ''}" data-rt-tab="core">
                <span>${escapeHtml(t.navCore)}</span>
                <span class="nav-count">${totalCore}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'stack' ? 'active' : ''}" data-rt-tab="stack">
                <span>${escapeHtml(t.navStack)}</span>
                <span class="nav-count">${totalStack}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'tools' ? 'active' : ''}" data-rt-tab="tools">
                <span>${escapeHtml(t.navTools)}</span>
                <span class="nav-count">${allCliSdk.length}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'pkgmgr' ? 'active' : ''}" data-rt-tab="pkgmgr">
                <span>${escapeHtml(t.navPkgMgr)}</span>
                <span class="nav-count">${allPkgMgr.length}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'path' ? 'active' : ''}" data-rt-tab="path">
                <span>${escapeHtml(t.navUserPath)}</span>
                <span class="nav-count">${s.userPathList.length}</span>
            </button>
        </div>

        <div class="active-summary">
            <div class="nav-section-title">${escapeHtml(t.envSectionTitle)}</div>
            <button
                class="sidebar-env-btn"
                id="btn-output-modal"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.outputChangeTooltip)}"
            >
                <div class="sidebar-env-btn-head">
                    <span>${escapeHtml(t.outputLabel)}</span>
                    <span style="color: var(--accent-blue)">✎</span>
                </div>
                <div class="sidebar-env-btn-val" title="${escapeHtml(s.baseDir)}">${escapeHtml(s.baseDir)}</div>
            </button>
            <div class="sidebar-env-row">
                <span class="sidebar-env-key">${escapeHtml(t.envPathScopeLabel)}</span>
                <span class="sidebar-env-val sidebar-env-val-accent">HKCU User PATH</span>
            </div>
            <div class="sidebar-env-row">
                <span class="sidebar-env-key">${escapeHtml(t.envLastCheckLabel)}</span>
                <span class="sidebar-env-val">${escapeHtml(s.lastLtsCheck || t.envCheckedNever)}</span>
            </div>
        </div>
    `;

    return { sidebarHtml, contentHtml };
}

function renderEssentialsView(s: main.DashboardState): { sidebarHtml: string; contentHtml: string } {
    const t = getT(currentLang());
    const allEss = s.essentials || [];
    const readyPkgMgrs = (s.pkgManagers || []).filter((m) => m.available);
    const readyPkgMgrTip = readyPkgMgrs.map((m) => `${m.name}${m.version ? ` v${m.version}` : ''}`).join(', ');

    const countCat = (cat: string) => allEss.filter((e) => e.category === cat).length;

    const allInstalled = s.installedApps || [];
    const activeInstalledCount = allInstalled.filter((a) => !a.ignored).length;
    const updatesCount = allInstalled.filter((a) => !a.ignored && a.hasUpdate).length;

    const activeConfigPath = essentialTab === 'installed' ? s.ignoredConfPath || s.essentialsConfPath : s.essentialsConfPath;
    const envBannerHtml = `
        <div class="env-banner">
            <div class="env-pills">
                ${
                    s.wingetEnv.wingetAvailable
                        ? `<span class="badge badge-active">${escapeHtml(t.essWingetReady(s.wingetEnv.wingetVersion))}</span>`
                        : `<span class="badge badge-warn">${escapeHtml(t.essWingetMissing)}</span>`
                }
                ${
                    s.wingetEnv.isAdmin
                        ? `<span class="badge badge-active">${escapeHtml(t.essUacAdmin)}</span>`
                        : `<span class="badge badge-warn">${escapeHtml(t.essUacStandard)}</span>`
                }
                <span class="card-id" title="${escapeHtml(activeConfigPath)}">${escapeHtml(t.essConfigStoredHint(activeConfigPath))}</span>
            </div>
            <div class="env-actions">
                ${
                    !s.wingetEnv.wingetAvailable
                        ? `<button class="btn btn-primary btn-sm" id="btn-install-winget" ${busy ? 'disabled' : ''}>${escapeHtml(t.essInstallWinget)}</button>`
                        : ''
                }
                ${
                    !s.wingetEnv.isAdmin
                        ? `<button class="btn btn-sm" id="btn-restart-admin" ${busy ? 'disabled' : ''}>${escapeHtml(t.essRestartAdmin)}</button>`
                        : ''
                }
            </div>
        </div>
    `;

    let contentHtml = envBannerHtml;
    if (essentialTab === 'search') {
        contentHtml += renderWingetSearchView();
    } else if (essentialTab === 'installed') {
        contentHtml += renderInstalledWorkspaceView(s);
    } else {
        const filtered = allEss.filter((e) => {
            if (essentialTab !== 'all' && e.category !== essentialTab) return false;
            return matchesSearch(`${e.name} ${e.id} ${e.wingetId} ${e.description}`, essentialSearchQuery);
        });

        const fallbackSearchBtn =
            essentialSearchQuery.trim() && filtered.length === 0
                ? `<div style="margin-top:10px"><button class="btn btn-primary btn-sm" id="btn-jump-winget-search">${escapeHtml(t.essSearchInWinget(essentialSearchQuery.trim()))}</button></div>`
                : '';

        contentHtml += `
            <section>
                <div class="section-header">
                    <span class="section-title">${escapeHtml(t.tabWindowsApps)} (${filtered.length})</span>
                    <span class="section-subtitle">${escapeHtml(t.essSecSubtitle)}</span>
                </div>
                <div class="cards-grid">
                    ${
                        filtered.length > 0
                            ? filtered.map(renderEssentialCard).join('')
                            : `<div class="empty-state">${escapeHtml(t.essEmpty)}${fallbackSearchBtn}</div>`
                    }
                </div>
            </section>
        `;
    }

    const sidebarHtml = `
        <input class="search-input" id="ess-search-input" type="text" placeholder="${escapeHtml(t.searchEssPlaceholder)}" value="${escapeHtml(essentialSearchQuery)}" />
        <div class="nav-section-title">${escapeHtml(t.navWorkspace)}</div>
        <div class="nav-list">
            <button class="nav-item ${essentialTab === 'all' ? 'active' : ''}" data-es-tab="all">
                <span>${escapeHtml(t.essCatAll)}</span>
                <span class="nav-count">${allEss.length}</span>
            </button>
            <button class="nav-item ${essentialTab === 'browsers' ? 'active' : ''}" data-es-tab="browsers">
                <span>${escapeHtml(t.essCatBrowsers)}</span>
                <span class="nav-count">${countCat('browsers')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'utilities' ? 'active' : ''}" data-es-tab="utilities">
                <span>${escapeHtml(t.essCatUtilities)}</span>
                <span class="nav-count">${countCat('utilities')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'editors' ? 'active' : ''}" data-es-tab="editors">
                <span>${escapeHtml(t.essCatEditors)}</span>
                <span class="nav-count">${countCat('editors')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'office' ? 'active' : ''}" data-es-tab="office">
                <span>${escapeHtml(t.essCatOffice)}</span>
                <span class="nav-count">${countCat('office')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'chat' ? 'active' : ''}" data-es-tab="chat">
                <span>${escapeHtml(t.essCatChat)}</span>
                <span class="nav-count">${countCat('chat')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'downloaders' ? 'active' : ''}" data-es-tab="downloaders">
                <span>${escapeHtml(t.essCatDownloaders)}</span>
                <span class="nav-count">${countCat('downloaders')}</span>
            </button>
            <button class="nav-item ${essentialTab === 'installed' ? 'active' : ''}" data-es-tab="installed">
                <span>${escapeHtml(t.essCatInstalled)}</span>
                <span class="nav-count ${updatesCount > 0 ? 'nav-count-warn' : ''}">${updatesCount > 0 ? updatesCount : activeInstalledCount}</span>
            </button>
            <button class="nav-item ${essentialTab === 'search' ? 'active' : ''}" data-es-tab="search">
                <span>${escapeHtml(t.essCatSearch)}</span>
                <span class="nav-count">⌕</span>
            </button>
        </div>

        <div class="active-summary">
            <div class="nav-section-title">${escapeHtml(t.envSectionTitle)}</div>
            <div class="sidebar-env-row">
                <span class="sidebar-env-key">${escapeHtml(t.envWingetLabel)}</span>
                <span class="sidebar-env-val ${s.wingetEnv.wingetAvailable ? 'sidebar-env-val-ok' : 'sidebar-env-val-warn'}">
                    ${escapeHtml(s.wingetEnv.wingetAvailable ? s.wingetEnv.wingetVersion || 'Ready' : t.envMissingLabel)}
                </span>
            </div>
            <div class="sidebar-env-row" title="${escapeHtml(readyPkgMgrTip)}">
                <span class="sidebar-env-key">${escapeHtml(t.envPkgMgrLabel)}</span>
                <span class="sidebar-env-val sidebar-env-val-accent">${escapeHtml(t.pkgManagersPill(readyPkgMgrs.length))}</span>
            </div>
            <div class="sidebar-env-row">
                <span class="sidebar-env-key">${escapeHtml(t.envPrivilegeLabel)}</span>
                <span class="sidebar-env-val ${s.wingetEnv.isAdmin ? 'sidebar-env-val-ok' : 'sidebar-env-val-warn'}">
                    ${escapeHtml(s.wingetEnv.isAdmin ? t.envPrivilegeAdmin : t.envPrivilegeStandard)}
                </span>
            </div>
        </div>
    `;

    return { sidebarHtml, contentHtml };
}

function windowMaximizeIcon(isMax: boolean): string {
    if (isMax) {
        return `<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.2"><rect x="2" y="3.5" width="6.5" height="6.5" rx="0.8"/><path d="M4 3.5V2.2a.7.7 0 0 1 .7-.7h5.1a.7.7 0 0 1 .7.7v5.1a.7.7 0 0 1-.7.7H8.5"/></svg>`;
    }
    return `<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.2"><rect x="2" y="2" width="8" height="8" rx="1"/></svg>`;
}

function renderWindowControls(lang: Lang): string {
    const t = getT(lang);
    const maxTip = windowMaximized ? t.winRestoreTip : t.winMaximizeTip;
    return `
        <div class="window-controls">
            <button class="win-btn" id="btn-win-min" title="${escapeHtml(t.winMinimizeTip)}" aria-label="${escapeHtml(t.winMinimizeTip)}">
                <svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.25"><path d="M2 6h8"/></svg>
            </button>
            <button class="win-btn" id="btn-win-max" title="${escapeHtml(maxTip)}" aria-label="${escapeHtml(maxTip)}">
                ${windowMaximizeIcon(windowMaximized)}
            </button>
            <button class="win-btn win-btn-close" id="btn-win-close" title="${escapeHtml(t.winCloseTip)}" aria-label="${escapeHtml(t.winCloseTip)}">
                <svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round"><path d="M2.5 2.5l7 7M9.5 2.5l-7 7"/></svg>
            </button>
        </div>
    `;
}

function updateWindowControlDom(): void {
    const t = getT(currentLang());
    const rootEl = document.getElementById('app');
    rootEl?.classList.toggle('window-maximized', windowMaximized);
    const maxBtn = document.getElementById('btn-win-max');
    if (maxBtn) {
        const tip = windowMaximized ? t.winRestoreTip : t.winMaximizeTip;
        maxBtn.title = tip;
        maxBtn.setAttribute('aria-label', tip);
        maxBtn.innerHTML = windowMaximizeIcon(windowMaximized);
    }
}

function syncWindowMaximizedState(): void {
    WindowIsMaximised()
        .then((isMax) => {
            windowMaximized = Boolean(isMax);
            updateWindowControlDom();
        })
        .catch(() => {
            // Safe to ignore in headless browser preview without Wails runtime.
        });
}

function bindWindowControlListeners(): void {
    document.getElementById('btn-win-min')?.addEventListener('click', () => {
        MinimizeWindow().catch(() => WindowMinimise());
    });
    document.getElementById('btn-win-max')?.addEventListener('click', () => {
        WindowToggleMaximise();
        setTimeout(syncWindowMaximizedState, 60);
    });
    document.getElementById('btn-win-close')?.addEventListener('click', () => {
        const act = state?.settings?.closeAction || 'ask';
        if (act === 'tray') {
            HideToTray().catch(() => Quit());
            return;
        }
        if (act === 'quit') {
            QuitApp().catch(() => Quit());
            return;
        }
        closeConfirmRemember = false;
        showCloseConfirmModal = true;
        render(true);
    });
}

function render(skipScrollCapture = false): void {
    const lang = currentLang();
    const t = getT(lang);

    if (!state) {
        appRoot.innerHTML = `
            <header class="topbar">
                <div class="brand">
                    <div class="brand-mark">&gt;_</div>
                    <span class="brand-title">UniDevUI</span>
                </div>
                <div class="topbar-actions">
                    ${renderWindowControls(lang)}
                </div>
            </header>
            <div class="empty-state" style="margin:auto">${escapeHtml(t.loading)}</div>
        `;
        bindWindowControlListeners();
        updateWindowControlDom();
        return;
    }

    const focusedEl = document.activeElement as HTMLElement | null;
    const focusedInputId =
        focusedEl && (focusedEl.id === 'winget-search-input' || focusedEl.id === 'search-input' || focusedEl.id === 'ess-search-input')
            ? focusedEl.id
            : '';
    const focusedSelStart =
        focusedInputId && focusedEl instanceof HTMLInputElement ? (focusedEl.selectionStart ?? focusedEl.value.length) : 0;
    const focusedSelEnd =
        focusedInputId && focusedEl instanceof HTMLInputElement ? (focusedEl.selectionEnd ?? focusedSelStart) : focusedSelStart;

    if (!skipScrollCapture) {
        const prevContent = document.querySelector<HTMLElement>('.content');
        if (prevContent) {
            scrollByViewKey[currentViewKey()] = prevContent.scrollTop;
        }
        const prevSummary = document.querySelector<HTMLElement>('.active-summary');
        if (prevSummary) {
            sidebarSummaryScrollTop = prevSummary.scrollTop;
        }
    }

    const { sidebarHtml, contentHtml } =
        primaryTab === 'runtimes' ? renderRuntimesView(state) : renderEssentialsView(state);

    const selectedEssCount = (state.essentials || []).filter((e) => Boolean(checkedEssentials[e.id])).length;

    const topbarRightActions =
        primaryTab === 'runtimes'
            ? `
                <button class="btn" id="btn-packages" title="${escapeHtml(t.btnPackagesTooltip)}">${escapeHtml(t.btnPackages)}</button>
                <button class="btn btn-primary" id="btn-update-lts" ${busy ? 'disabled' : ''} title="${escapeHtml(t.btnCheckUpdatesTooltip)}">${escapeHtml(t.btnCheckUpdates)}</button>
                <button class="btn" id="btn-reload" title="${escapeHtml(t.btnReloadTooltip)}">${escapeHtml(t.btnReload)}</button>
                <button class="btn" id="btn-open-dir" title="${escapeHtml(t.btnOpenFolderTooltip)}">${escapeHtml(t.btnOpenFolder)}</button>
            `
            : `
                <button class="btn" id="btn-essentials-conf" title="${escapeHtml(t.btnEssentialsConfTooltip)}">${escapeHtml(t.btnEssentialsConf)}</button>
                <button class="btn" id="btn-refresh-winget" ${busy ? 'disabled' : ''}>${escapeHtml(t.btnRefreshWinget)}</button>
                <button
                    class="btn btn-primary"
                    id="btn-topbar-install-batch"
                    ${busy || selectedEssCount === 0 || !state.wingetEnv.wingetAvailable ? 'disabled' : ''}
                >
                    ${escapeHtml(t.essInstallSelected(selectedEssCount))}
                </button>
            `;

    appRoot.innerHTML = `
        <header class="topbar">
            <div class="brand">
                <div class="brand-mark">&gt;_</div>
                <span class="brand-title">UniDevUI</span>
                <div class="mode-tabs">
                    <button class="mode-tab ${primaryTab === 'runtimes' ? 'active' : ''}" data-primary-tab="runtimes">${escapeHtml(t.tabRuntimes)}</button>
                    <button class="mode-tab ${primaryTab === 'essentials' ? 'active' : ''}" data-primary-tab="essentials">${escapeHtml(t.tabWindowsApps)}</button>
                </div>
            </div>
            <div class="topbar-actions">
                <div class="lang-switch" title="Language / Bahasa">
                    <button class="lang-btn ${lang === 'en' ? 'active' : ''}" data-lang="en">EN</button>
                    <button class="lang-btn ${lang === 'id' ? 'active' : ''}" data-lang="id">ID</button>
                </div>
                ${topbarRightActions}
                <button
                    class="${state.appUpdate?.hasUpdate ? 'btn btn-update-ready' : 'btn'}"
                    id="btn-about"
                    title="${escapeHtml(state.appUpdate?.hasUpdate ? t.aboutUpdateAvailableTip(state.appUpdate.latestVersion) : t.btnAboutTooltip)}"
                >ⓘ ${escapeHtml(t.btnAbout)}${state.appUpdate?.hasUpdate ? ` <span class="update-pill">${escapeHtml(t.aboutUpdateAvailableBadge(state.appUpdate.latestVersion))}</span>` : ''}</button>
                <button
                    class="btn btn-topbar-icon"
                    id="btn-settings"
                    title="${escapeHtml(t.btnSettingsTooltip)}"
                    aria-label="${escapeHtml(t.settingsTitle)}"
                >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
                </button>
                ${renderWindowControls(lang)}
            </div>
        </header>

        <div class="workspace">
            <aside class="sidebar">
                ${sidebarHtml}
            </aside>
            <div class="main-pane">
                <main class="content">
                    ${contentHtml}
                </main>
                <section class="terminal-drawer ${terminalCollapsed ? 'collapsed' : ''}">
                    <div class="terminal-header">
                        <div class="terminal-title-group">
                            <span class="terminal-label">${escapeHtml(t.terminalTitle)}</span>
                            <div id="progress-slot">${renderProgressInline()}</div>
                        </div>
                        <div class="terminal-actions">
                            ${busy ? `<button class="btn btn-danger btn-sm" id="btn-cancel">${escapeHtml(t.btnCancel)}</button>` : ''}
                            <button
                                class="btn btn-sm btn-term-icon"
                                id="btn-clear-log"
                                title="${escapeHtml(t.btnClear)}"
                                aria-label="${escapeHtml(t.btnClear)}"
                            >
                                <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 4.5h11M6 4.5V3a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1.5M4.5 4.5l.7 8.5a1 1 0 0 0 1 .9h3.6a1 1 0 0 0 1-.9l.7-8.5"/><path d="M6.75 7.25v4M9.25 7.25v4"/></svg>
                            </button>
                            <button
                                class="btn btn-sm btn-term-icon"
                                id="btn-toggle-term"
                                title="${escapeHtml(terminalCollapsed ? t.btnExpand : t.btnMinimize)}"
                                aria-label="${escapeHtml(terminalCollapsed ? t.btnExpand : t.btnMinimize)}"
                            >
                                ${
                                    terminalCollapsed
                                        ? `<svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 10.5L8 6l4.5 4.5"/></svg>`
                                        : `<svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 5.5L8 10l4.5-4.5"/></svg>`
                                }
                            </button>
                        </div>
                    </div>
                    <div class="terminal-body" id="terminal-body"></div>
                </section>
            </div>
        </div>

        ${renderSettingsModal(state)}
        ${renderCloseConfirmModal()}
        ${renderDetailModal()}
        ${renderAboutModal(state)}
    `;

    renderTerminalLinesOnly();
    bindEventListeners();
    updateWindowControlDom();
    if (focusedInputId) {
        const freshInput = document.getElementById(focusedInputId) as HTMLInputElement | null;
        if (freshInput) {
            freshInput.focus();
            freshInput.setSelectionRange(focusedSelStart, focusedSelEnd);
        }
    }
    if (primaryTab === 'essentials') {
        hydrateCustomEssentialsIfNeeded(state.essentials || []);
        if (essentialTab === 'installed') {
            hydrateInstalledAppsIfNeeded(state.installedApps || []);
        }
    }

    const nextContent = document.querySelector<HTMLElement>('.content');
    if (nextContent) {
        nextContent.scrollTop = scrollByViewKey[currentViewKey()] || 0;
        nextContent.addEventListener(
            'scroll',
            () => {
                scrollByViewKey[currentViewKey()] = nextContent.scrollTop;
            },
            { passive: true },
        );
    }
    const nextSummary = document.querySelector<HTMLElement>('.active-summary');
    if (nextSummary) {
        nextSummary.scrollTop = sidebarSummaryScrollTop;
        nextSummary.addEventListener(
            'scroll',
            () => {
                sidebarSummaryScrollTop = nextSummary.scrollTop;
            },
            { passive: true },
        );
    }
}

function updateProgressSlotOnly(): void {
    const slot = document.getElementById('progress-slot');
    if (slot) {
        slot.innerHTML = renderProgressInline();
    }
}

function captureCurrentScroll(): void {
    const prevContent = document.querySelector<HTMLElement>('.content');
    if (prevContent) {
        scrollByViewKey[currentViewKey()] = prevContent.scrollTop;
    }
    const prevSummary = document.querySelector<HTMLElement>('.active-summary');
    if (prevSummary) {
        sidebarSummaryScrollTop = prevSummary.scrollTop;
    }
}

function triggerBatchInstall(): void {
    if (!state) return;
    const queue: main.BatchEssentialItem[] = [];
    for (const item of state.essentials || []) {
        if (checkedEssentials[item.id]) {
            queue.push(
                main.BatchEssentialItem.createFrom({
                    id: item.id,
                    version: selectedEssentialVerByApp[item.id] || 'latest',
                }),
            );
        }
    }
    if (queue.length === 0) return;
    InstallSelectedEssentials(queue).catch((e) => reportErr(e));
}

function clearUnifiedCatalogSearch(): void {
    wingetSearchToken++;
    wingetCatalogQuery = '';
    wingetSearchResults = [];
    extraSearchByGroup = {};
    unifiedJSMgrs = [];
    unifiedPyMgrs = [];
    unifiedSearchPending = [];
    unifiedSearchTotal = 0;
    unifiedSearchDone = false;
    searchResultFilter = 'all';
    wingetSearching = false;
    wingetEnriching = false;
    wingetSearchExecuted = false;
    CancelUnifiedSearch().catch(() => {
        // Ignore cancellation error on cleanup.
    });
    render();
}

function runWingetCatalogSearch(queryOverride?: string): void {
    const q = (queryOverride ?? wingetCatalogQuery).trim();
    if (!q) return;
    wingetCatalogQuery = q;
    const token = ++wingetSearchToken;
    searchResultFilter = 'all';
    wingetSearchResults = [];
    extraSearchByGroup = {};
    unifiedJSMgrs = [];
    unifiedPyMgrs = [];
    unifiedSearchPending = [];
    unifiedSearchTotal = 0;
    unifiedSearchDone = false;
    wingetSearching = true;
    wingetEnriching = false;
    wingetSearchExecuted = true;
    render();

    StartUnifiedSearch(token, q)
        .then((startInfo) => {
            if (token !== wingetSearchToken) return;
            unifiedSearchTotal = startInfo.total || 0;
            if ((startInfo.jsMgrs || []).length > 0 && unifiedJSMgrs.length === 0) {
                unifiedJSMgrs = startInfo.jsMgrs as ManagerKey[];
            }
            if ((startInfo.pyMgrs || []).length > 0 && unifiedPyMgrs.length === 0) {
                unifiedPyMgrs = startInfo.pyMgrs as ManagerKey[];
            }
            if (!startInfo.total) {
                wingetSearching = false;
                unifiedSearchDone = true;
                unifiedSearchPending = [];
                render();
                return;
            }
            if (!unifiedSearchDone && unifiedSearchPending.length === 0 && (startInfo.sources || []).length > 0) {
                unifiedSearchPending = startInfo.sources;
                render(true);
            }
        })
        .catch((e) => {
            if (token !== wingetSearchToken) return;
            wingetSearching = false;
            unifiedSearchDone = true;
            unifiedSearchPending = [];
            reportErr(e);
            render();
        });
}

function bindEventListeners(): void {
    bindWindowControlListeners();
    document.querySelectorAll<HTMLButtonElement>('[data-primary-tab]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const nextPrimary = btn.dataset.primaryTab as PrimaryTab | undefined;
            if (nextPrimary && nextPrimary !== primaryTab) {
                captureCurrentScroll();
                primaryTab = nextPrimary;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-lang]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const nextLang = btn.dataset.lang;
            if (nextLang && nextLang !== currentLang()) {
                SetLanguage(nextLang)
                    .then((nextState) => {
                        state = nextState;
                        render();
                    })
                    .catch((e) => reportErr(e));
            }
        });
    });

    document.getElementById('btn-output-modal')?.addEventListener('click', openSettingsModal);
    document.getElementById('btn-settings')?.addEventListener('click', openSettingsModal);

    const closeSettingsModal = (): void => {
        showSettingsModal = false;
        settingsDraft = null;
        render(true);
    };
    document.getElementById('btn-settings-close')?.addEventListener('click', closeSettingsModal);
    document.getElementById('btn-settings-cancel')?.addEventListener('click', closeSettingsModal);
    document.getElementById('settings-modal-backdrop')?.addEventListener('click', (ev) => {
        if (ev.target === ev.currentTarget) closeSettingsModal();
    });

    const closeConfirmModal = (): void => {
        showCloseConfirmModal = false;
        render(true);
    };
    document.getElementById('btn-close-confirm-x')?.addEventListener('click', closeConfirmModal);
    document.getElementById('btn-close-cancel')?.addEventListener('click', closeConfirmModal);
    document.getElementById('close-confirm-backdrop')?.addEventListener('click', (ev) => {
        if (ev.target === ev.currentTarget) closeConfirmModal();
    });
    const rememberEl = document.getElementById('chk-close-remember') as HTMLInputElement | null;
    rememberEl?.addEventListener('change', () => {
        closeConfirmRemember = rememberEl.checked;
    });
    document.getElementById('btn-close-tray')?.addEventListener('click', () => {
        showCloseConfirmModal = false;
        render(true);
        ConfirmCloseAction('tray', closeConfirmRemember).catch((e) => reportErr(e));
    });
    document.getElementById('btn-close-quit')?.addEventListener('click', () => {
        showCloseConfirmModal = false;
        render(true);
        ConfirmCloseAction('quit', closeConfirmRemember).catch((e) => reportErr(e));
    });

    document.getElementById('btn-about')?.addEventListener('click', () => {
        showAboutModal = true;
        render(true);
    });

    const closeAboutModal = (): void => {
        showAboutModal = false;
        render(true);
    };
    document.getElementById('btn-about-close')?.addEventListener('click', closeAboutModal);
    document.getElementById('btn-about-close-footer')?.addEventListener('click', closeAboutModal);
    document.getElementById('about-modal-backdrop')?.addEventListener('click', (ev) => {
        if (ev.target === ev.currentTarget) closeAboutModal();
    });

    document.getElementById('btn-app-check-update')?.addEventListener('click', () => {
        if (checkingAppUpdate) return;
        checkingAppUpdate = true;
        render(true);
        CheckAppUpdate()
            .then((nextState) => {
                state = nextState;
            })
            .catch((e) => reportErr(e))
            .finally(() => {
                checkingAppUpdate = false;
                render(true);
            });
    });

    document.getElementById('btn-app-self-update')?.addEventListener('click', () => {
        showAboutModal = false;
        render(true);
        ApplyAppUpdate().catch((e) => reportErr(e));
    });

    const setLangEl = document.getElementById('set-lang-select') as HTMLSelectElement | null;
    setLangEl?.addEventListener('change', () => {
        if (settingsDraft) settingsDraft.language = setLangEl.value;
    });

    const setBaseEl = document.getElementById('set-basedir-input') as HTMLInputElement | null;
    setBaseEl?.addEventListener('input', () => {
        if (settingsDraft) settingsDraft.baseDir = setBaseEl.value;
    });
    setBaseEl?.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter') {
            document.getElementById('btn-settings-save')?.click();
        }
    });

    document.getElementById('btn-set-browse')?.addEventListener('click', () => {
        PickOutputDirectory()
            .then((picked) => {
                if (picked && settingsDraft) {
                    settingsDraft.baseDir = picked;
                    render(true);
                }
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-set-reset')?.addEventListener('click', () => {
        if (settingsDraft) {
            settingsDraft.baseDir = 'C:\\AppPath';
            render(true);
        }
    });

    document.getElementById('btn-set-sync')?.addEventListener('click', () => {
        SyncDefaultPackages()
            .then((nextState) => {
                state = nextState;
                showSettingsModal = false;
                settingsDraft = null;
                render();
            })
            .catch((e) => reportErr(e));
    });

    const setCloseEl = document.getElementById('set-close-action') as HTMLSelectElement | null;
    setCloseEl?.addEventListener('change', () => {
        if (settingsDraft) settingsDraft.closeAction = setCloseEl.value;
    });

    const setAutoStartEl = document.getElementById('chk-set-autostart') as HTMLInputElement | null;
    setAutoStartEl?.addEventListener('change', () => {
        if (settingsDraft) {
            settingsDraft.autoStart = setAutoStartEl.checked;
            render(true);
        }
    });

    const setStartTrayEl = document.getElementById('chk-set-start-tray') as HTMLInputElement | null;
    setStartTrayEl?.addEventListener('change', () => {
        if (settingsDraft) settingsDraft.startInTray = setStartTrayEl.checked;
    });

    const setAutoCheckEl = document.getElementById('chk-set-autocheck') as HTMLInputElement | null;
    setAutoCheckEl?.addEventListener('change', () => {
        if (settingsDraft) {
            settingsDraft.autoCheckUpdates = setAutoCheckEl.checked;
            render(true);
        }
    });

    const setIntervalEl = document.getElementById('set-check-interval') as HTMLSelectElement | null;
    setIntervalEl?.addEventListener('change', () => {
        if (settingsDraft) settingsDraft.autoCheckIntervalHours = parseInt(setIntervalEl.value, 10) || 6;
    });

    const setDelayScanEl = document.getElementById('chk-set-delay-scan') as HTMLInputElement | null;
    setDelayScanEl?.addEventListener('change', () => {
        if (settingsDraft) settingsDraft.delayStartupScan = setDelayScanEl.checked;
    });

    document.getElementById('btn-settings-save')?.addEventListener('click', () => {
        if (!state) return;
        const draft = getEffectiveSettingsDraft(state);
        if (setBaseEl) draft.baseDir = setBaseEl.value;
        SaveAppSettings(draft)
            .then((nextState) => {
                state = nextState;
                showSettingsModal = false;
                settingsDraft = null;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-packages')?.addEventListener('click', () => {
        OpenPackagesConf().catch((e) => reportErr(e));
    });

    document.getElementById('btn-essentials-conf')?.addEventListener('click', () => {
        OpenEssentialsConf().catch((e) => reportErr(e));
    });

    document.getElementById('btn-update-lts')?.addEventListener('click', () => {
        UpdateLTSLinks().catch((e) => reportErr(e));
    });

    document.getElementById('btn-reload')?.addEventListener('click', () => {
        ReloadConfig()
            .then((nextState) => {
                state = nextState;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-refresh-winget')?.addEventListener('click', () => {
        RefreshWingetStatus()
            .then((nextState) => {
                state = nextState;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-open-dir')?.addEventListener('click', () => {
        OpenAppPathFolder().catch((e) => reportErr(e));
    });

    document.getElementById('btn-install-winget')?.addEventListener('click', () => {
        InstallWingetCLI().catch((e) => reportErr(e));
    });

    document.getElementById('btn-restart-admin')?.addEventListener('click', () => {
        RestartAsAdmin().catch((e) => reportErr(e));
    });

    document.getElementById('btn-cancel')?.addEventListener('click', () => {
        CancelActiveTask().catch((e) => reportErr(e));
    });

    document.getElementById('btn-clear-log')?.addEventListener('click', () => {
        logs = [];
        renderTerminalLinesOnly();
    });

    document.getElementById('btn-toggle-term')?.addEventListener('click', () => {
        terminalCollapsed = !terminalCollapsed;
        render();
    });

    const searchEl = document.getElementById('search-input') as HTMLInputElement | null;
    searchEl?.addEventListener('input', (ev) => {
        const target = ev.target as HTMLInputElement;
        const pos = target.selectionStart ?? target.value.length;
        runtimeSearchQuery = target.value;
        render();
        const fresh = document.getElementById('search-input') as HTMLInputElement | null;
        if (fresh) {
            fresh.focus();
            fresh.setSelectionRange(pos, pos);
        }
    });

    const essSearchEl = document.getElementById('ess-search-input') as HTMLInputElement | null;
    essSearchEl?.addEventListener('input', (ev) => {
        const target = ev.target as HTMLInputElement;
        const pos = target.selectionStart ?? target.value.length;
        essentialSearchQuery = target.value;
        render();
        const fresh = document.getElementById('ess-search-input') as HTMLInputElement | null;
        if (fresh) {
            fresh.focus();
            fresh.setSelectionRange(pos, pos);
        }
    });

    const wingetInputEl = document.getElementById('winget-search-input') as HTMLInputElement | null;
    wingetInputEl?.addEventListener('input', (ev) => {
        wingetCatalogQuery = (ev.target as HTMLInputElement).value;
    });
    wingetInputEl?.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter') {
            runWingetCatalogSearch();
        }
    });
    document.getElementById('btn-winget-search')?.addEventListener('click', () => {
        runWingetCatalogSearch();
    });
    document.getElementById('btn-winget-search-clear')?.addEventListener('click', () => {
        clearUnifiedCatalogSearch();
    });

    document.getElementById('btn-jump-winget-search')?.addEventListener('click', () => {
        const q = essentialSearchQuery.trim();
        essentialTab = 'search';
        runWingetCatalogSearch(q);
    });

    document.querySelectorAll<HTMLButtonElement>('[data-rt-tab]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const tab = btn.dataset.rtTab as RuntimeTabKey | undefined;
            if (tab && tab !== runtimeTab) {
                captureCurrentScroll();
                runtimeTab = tab;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-es-tab]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const tab = btn.dataset.esTab as EssentialTabKey | undefined;
            if (tab && tab !== essentialTab) {
                captureCurrentScroll();
                essentialTab = tab;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-installed-filter]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const nextFilter = btn.dataset.installedFilter as InstalledFilterKey | undefined;
            if (nextFilter && nextFilter !== installedFilter) {
                captureCurrentScroll();
                installedFilter = nextFilter;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-installed-mgr]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const nextMgr = btn.dataset.installedMgr as InstalledMgrFilterKey | undefined;
            if (nextMgr && nextMgr !== installedMgrFilter) {
                captureCurrentScroll();
                installedMgrFilter = nextMgr;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-search-filter]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const nextFilter = btn.dataset.searchFilter as SearchFilterKey | undefined;
            if (nextFilter && nextFilter !== searchResultFilter) {
                captureCurrentScroll();
                searchResultFilter = nextFilter;
                render(true);
            }
        });
    });

    document.querySelectorAll<HTMLSelectElement>('[data-role="search-js-mgr"]').forEach((sel) => {
        sel.addEventListener('change', () => {
            const pkgKey = sel.dataset.pkgKey;
            if (pkgKey) {
                selectedSearchJSMgrByPkg[pkgKey] = sel.value as ManagerKey;
                render(true);
            }
        });
    });

    document.getElementById('btn-ignored-conf')?.addEventListener('click', () => {
        OpenIgnoredConf().catch((e) => reportErr(e));
    });

    document.getElementById('btn-update-all-installed')?.addEventListener('click', () => {
        const targets = (state?.installedApps || [])
            .filter(
                (a) =>
                    !a.ignored &&
                    a.hasUpdate &&
                    a.canUpdate &&
                    (installedMgrFilter === 'all' || (a.manager || 'winget').toLowerCase() === installedMgrFilter),
            )
            .map((a) =>
                main.WingetUpdateTarget.createFrom({
                    manager: a.manager || 'winget',
                    name: a.name,
                    wingetId: a.id,
                    source: a.source || a.manager || 'winget',
                    availableVer: a.availableVer || '',
                }),
            );
        if (targets.length > 0) {
            UpdateAllInstalled(targets).catch((e) => reportErr(e));
        }
    });

    document.getElementById('btn-path-add')?.addEventListener('click', () => {
        PickAndAddUserPathDir()
            .then((nextState) => {
                state = nextState;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-path-clean')?.addEventListener('click', () => {
        CleanInvalidUserPath()
            .then((nextState) => {
                state = nextState;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-path-refresh')?.addEventListener('click', () => {
        RefreshUserPath()
            .then((nextState) => {
                state = nextState;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-topbar-install-batch')?.addEventListener('click', () => {
        triggerBatchInstall();
    });

    document.querySelectorAll<HTMLInputElement>('[data-role="ess-check"]').forEach((chk) => {
        chk.addEventListener('change', () => {
            const id = chk.dataset.ess;
            if (id) {
                checkedEssentials[id] = chk.checked;
                render();
            }
        });
    });

    document.querySelectorAll<HTMLSelectElement>('[data-role="ver-select"]').forEach((sel) => {
        sel.addEventListener('change', () => {
            const appID = sel.dataset.app;
            if (appID) {
                selectedVersionByApp[appID] = sel.value;
                render();
            }
        });
    });

    document.querySelectorAll<HTMLSelectElement>('[data-role="ess-ver-select"]').forEach((sel) => {
        sel.addEventListener('change', () => {
            const essID = sel.dataset.ess;
            if (essID) {
                selectedEssentialVerByApp[essID] = sel.value;
                render();
            }
        });
    });

    document.querySelectorAll<HTMLButtonElement>('[data-action]').forEach((btn) => {
        btn.addEventListener('click', () => {
            const action = btn.dataset.action;
            const appID = btn.dataset.app || '';
            const ver = appID ? selectedVersionByApp[appID] || '' : '';

            if (action === 'download' && appID && ver) {
                DownloadAndInstall(appID, ver).catch((e) => reportErr(e));
            } else if (action === 'add-path' && appID && ver) {
                AddToPath(appID, ver)
                    .then((nextState) => {
                        state = nextState;
                        render();
                    })
                    .catch((e) => reportErr(e));
            } else if (action === 'remove-path' && appID) {
                RemoveFromPath(appID)
                    .then((nextState) => {
                        state = nextState;
                        render();
                    })
                    .catch((e) => reportErr(e));
            } else if (action === 'delete-ver' && appID && ver) {
                DeleteInstalledVersion(appID, ver)
                    .then((nextState) => {
                        state = nextState;
                        render();
                    })
                    .catch((e) => reportErr(e));
            } else if (action === 'install-tool') {
                const toolID = btn.dataset.tool || '';
                const mode = btn.dataset.mode || 'install';
                if (toolID) {
                    InstallCLITool(toolID, mode).catch((e) => reportErr(e));
                }
            } else if (action === 'add-custom-path') {
                const dir = btn.dataset.dir || '';
                if (dir) {
                    AddCustomDirToPath(dir)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'path-move') {
                const idx = Number.parseInt(btn.dataset.idx || '-1', 10);
                const delta = Number.parseInt(btn.dataset.delta || '0', 10);
                if (!Number.isNaN(idx) && idx >= 0 && delta !== 0) {
                    MoveUserPathEntry(idx, delta)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'path-open') {
                const dir = btn.dataset.dir || '';
                if (dir) {
                    OpenUserPathDir(dir).catch((e) => reportErr(e));
                }
            } else if (action === 'path-remove') {
                const idx = Number.parseInt(btn.dataset.idx || '-1', 10);
                const dir = btn.dataset.dir || '';
                if (!Number.isNaN(idx) && idx >= 0) {
                    RemoveUserPathEntry(idx, dir)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'ess-run') {
                const essID = btn.dataset.ess || '';
                const mode = btn.dataset.mode || 'install';
                const essVer = essID ? selectedEssentialVerByApp[essID] || 'latest' : 'latest';
                if (essID) {
                    RunEssentialTool(essID, mode, essVer).catch((e) => reportErr(e));
                }
            } else if (action === 'pkg-action') {
                const mgr = btn.dataset.mgr || 'winget';
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || '';
                const mode = btn.dataset.mode || 'install';
                if (wid) {
                    RunPackageAction(mgr, wid, name, source, mode).catch((e) => reportErr(e));
                }
            } else if (action === 'winget-install-raw') {
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || 'winget';
                if (wid) {
                    InstallWingetByID(name, wid, source).catch((e) => reportErr(e));
                }
            } else if (action === 'winget-update-raw') {
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || 'winget';
                if (wid) {
                    UpdateWingetByID(name, wid, source).catch((e) => reportErr(e));
                }
            } else if (action === 'winget-skip-ver') {
                const mgr = btn.dataset.mgr || 'winget';
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || '';
                const ver = btn.dataset.ver || '';
                if (wid && ver) {
                    SkipInstalledAppVersion(mgr, wid, name, source, ver)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'winget-pause-7d') {
                const mgr = btn.dataset.mgr || 'winget';
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || '';
                const days = Number(btn.dataset.days || '7') || 7;
                if (wid) {
                    PauseInstalledAppUpdates(mgr, wid, name, source, days)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'winget-ignore') {
                const mgr = btn.dataset.mgr || 'winget';
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || '';
                if (wid) {
                    IgnoreInstalledApp(mgr, wid, name, source, '')
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'winget-unignore') {
                const mgr = btn.dataset.mgr || 'winget';
                const wid = btn.dataset.wid || '';
                if (wid) {
                    UnignoreInstalledApp(mgr, wid)
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'winget-pin') {
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || 'winget';
                if (wid) {
                    PinWingetApp(name, wid, source, 'utilities')
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'ess-detail') {
                const wid = btn.dataset.wid || '';
                if (wid) {
                    openPackageDetailModal({
                        manager: btn.dataset.mgr || 'winget',
                        appId: btn.dataset.ess || undefined,
                        wingetId: wid,
                        name: btn.dataset.name || wid,
                        source: btn.dataset.source || 'winget',
                        version: btn.dataset.ver || undefined,
                        fallbackDesc: btn.dataset.desc || undefined,
                        iconUrl: btn.dataset.iconurl || undefined,
                        isEssentialItem: btn.dataset.isess === '1',
                    });
                }
            }
        });
    });

    document.querySelectorAll<HTMLImageElement>('.detail-shot-img').forEach((img) => {
        img.addEventListener('error', () => {
            const card = img.closest<HTMLElement>('.detail-shot-card');
            if (card) card.style.display = 'none';
        });
    });

    document.querySelectorAll<HTMLImageElement>('.app-icon-img').forEach((img) => {
        img.addEventListener('error', () => {
            img.style.display = 'none';
            const parent = img.parentElement;
            if (parent) {
                parent.classList.add('app-icon-fallback');
                const mono = parent.querySelector<HTMLElement>('.app-icon-mono');
                if (mono) mono.style.display = 'inline';
            }
        });
    });

    document.getElementById('btn-detail-close')?.addEventListener('click', () => {
        detailTarget = null;
        render(true);
    });
    document.getElementById('btn-detail-close-footer')?.addEventListener('click', () => {
        detailTarget = null;
        render(true);
    });
    document.getElementById('detail-modal-backdrop')?.addEventListener('click', (ev) => {
        if (ev.target === ev.currentTarget) {
            detailTarget = null;
            render(true);
        }
    });
    document.getElementById('btn-detail-install')?.addEventListener('click', () => {
        if (!detailTarget) return;
        const target = detailTarget;
        const mgr = (target.manager || 'winget').toLowerCase();
        detailTarget = null;
        if (mgr !== 'winget') {
            RunPackageAction(mgr, target.wingetId, target.name, target.source, 'install').catch((e) => reportErr(e));
        } else if (target.isEssentialItem && target.appId) {
            const ver = selectedEssentialVerByApp[target.appId] || 'latest';
            RunEssentialTool(target.appId, 'install', ver).catch((e) => reportErr(e));
        } else {
            InstallWingetByID(target.name, target.wingetId, target.source).catch((e) => reportErr(e));
        }
        render(true);
    });
    document.getElementById('btn-detail-pin')?.addEventListener('click', () => {
        if (!detailTarget) return;
        const target = detailTarget;
        PinWingetApp(target.name, target.wingetId, target.source || 'winget', 'utilities')
            .then((nextState) => {
                state = nextState;
                render(true);
            })
            .catch((e) => reportErr(e));
    });
    document.querySelectorAll<HTMLElement>('[data-open-url]').forEach((el) => {
        el.addEventListener('click', () => {
            const rawUrl = el.dataset.openUrl || '';
            if (rawUrl) {
                OpenExternalURL(rawUrl).catch((e) => reportErr(e));
            }
        });
    });
}

function reportErr(err: unknown): void {
    const msg = err instanceof Error ? err.message : String(err);
    appendLog(
        main.TerminalLogEvent.createFrom({
            timestamp: new Date().toTimeString().slice(0, 8),
            level: 'ERROR',
            message: msg,
        }),
    );
}

async function refreshState(): Promise<void> {
    try {
        const next = await GetDashboardState();
        state = next;
        busy = next.busy;
        if (!initialLogsLoaded && next.logs && next.logs.length > 0) {
            logs = [...next.logs];
            initialLogsLoaded = true;
        }
        render();
    } catch (err) {
        reportErr(err);
    }
}

EventsOn('terminal:log', (ev: main.TerminalLogEvent) => {
    appendLog(ev);
});

EventsOn('task:progress', (ev: ProgressState) => {
    progress = ev;
    updateProgressSlotOnly();
});

EventsOn('task:busy', (isBusy: boolean) => {
    busy = isBusy;
    if (!isBusy) {
        progress = { stage: 'idle', percent: 0, downloaded: 0, total: 0, speedBps: 0, detail: '' };
    }
    render();
});

EventsOn('state:refresh', () => {
    void refreshState();
});

EventsOn('pkgsearch:chunk', (ev: UnifiedSearchChunkEvent) => {
    if (!ev || ev.token !== wingetSearchToken) return;
    unifiedSearchTotal = ev.total || unifiedSearchTotal;
    unifiedSearchPending = ev.pending || [];
    unifiedSearchDone = Boolean(ev.done);
    if (ev.done) {
        wingetSearching = false;
    }

    if (ev.sourceGroup === 'winget') {
        const list = ev.wingetRows || [];
        wingetSearchResults = list;
        if (list.length > 0) {
            wingetEnriching = true;
            const curToken = ev.token;
            EnrichWingetSearch(list)
                .then((detailsList) => {
                    if (curToken !== wingetSearchToken) return;
                    for (let i = 0; i < (detailsList || []).length; i++) {
                        const det = detailsList[i];
                        const orig = list[i];
                        if (det && (det.id || orig?.id)) {
                            const k1 = detailKey(det.id || orig.id, det.source || orig?.source);
                            wingetDetailsByKey[k1] = det;
                            if (orig?.id) {
                                wingetDetailsByKey[detailKey(orig.id, orig.source)] = det;
                            }
                        }
                    }
                })
                .catch(() => {
                    // Search results remain usable even if metadata enrichment fails.
                })
                .finally(() => {
                    if (curToken === wingetSearchToken) {
                        wingetEnriching = false;
                        render(true);
                    }
                });
        }
    } else if (ev.sourceGroup) {
        extraSearchByGroup[ev.sourceGroup] = ev.pkgRows || [];
        if (ev.sourceGroup === 'js' && (ev.jsMgrs || []).length > 0) {
            unifiedJSMgrs = ev.jsMgrs as ManagerKey[];
        }
        if (ev.sourceGroup === 'py' && (ev.pyMgrs || []).length > 0) {
            unifiedPyMgrs = ev.pyMgrs as ManagerKey[];
        }
    }
    render(true);
});

window.addEventListener('keydown', (ev) => {
    if (ev.key !== 'Escape') return;
    if (showCloseConfirmModal) {
        showCloseConfirmModal = false;
        render(true);
    } else if (detailTarget) {
        detailTarget = null;
        render(true);
    } else if (showAboutModal) {
        showAboutModal = false;
        render(true);
    } else if (showSettingsModal) {
        showSettingsModal = false;
        settingsDraft = null;
        render(true);
    }
});

EventsOn('app:confirm-close', () => {
    closeConfirmRemember = false;
    showCloseConfirmModal = true;
    render(true);
});

EventsOn('nav:runtimes', () => {
    primaryTab = 'runtimes';
    runtimeTab = 'all';
    showAboutModal = false;
    showSettingsModal = false;
    showCloseConfirmModal = false;
    detailTarget = null;
    void refreshState();
});

EventsOn('nav:updates', () => {
    primaryTab = 'essentials';
    essentialTab = 'installed';
    installedFilter = 'updates';
    showAboutModal = false;
    showSettingsModal = false;
    showCloseConfirmModal = false;
    detailTarget = null;
    void refreshState();
});

EventsOn('nav:search', () => {
    primaryTab = 'essentials';
    essentialTab = 'search';
    showAboutModal = false;
    showSettingsModal = false;
    showCloseConfirmModal = false;
    detailTarget = null;
    void refreshState();
});

window.addEventListener('focus', () => {
    SyncPowerState().catch(() => {
        // Safe to ignore in headless preview without Wails runtime.
    });
});

let windowResizeRaf = 0;
window.addEventListener('resize', () => {
    if (windowResizeRaf) return;
    windowResizeRaf = window.requestAnimationFrame(() => {
        windowResizeRaf = 0;
        syncWindowMaximizedState();
    });
});

syncWindowMaximizedState();
void refreshState();
