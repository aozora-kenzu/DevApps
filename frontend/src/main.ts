import './style.css';
import {
    AddCustomDirToPath,
    AddToPath,
    CancelActiveTask,
    DeleteInstalledVersion,
    DownloadAndInstall,
    EnrichWingetSearch,
    GetDashboardState,
    GetWingetPackageDetails,
    IgnoreInstalledApp,
    InstallCLITool,
    InstallSelectedEssentials,
    InstallWingetByID,
    InstallWingetCLI,
    OpenAppPathFolder,
    OpenEssentialsConf,
    OpenExternalURL,
    OpenIgnoredConf,
    OpenPackagesConf,
    PickOutputDirectory,
    PinWingetApp,
    RefreshWingetStatus,
    ReloadConfig,
    RemoveFromPath,
    RestartAsAdmin,
    RunEssentialTool,
    SearchWingetApps,
    SetLanguage,
    SetOutputDir,
    SyncDefaultPackages,
    UnignoreInstalledApp,
    UpdateAllInstalled,
    UpdateLTSLinks,
    UpdateWingetByID,
} from '../wailsjs/go/main/App';
import { essentials, main, tools } from '../wailsjs/go/models';
import { EventsOn } from '../wailsjs/runtime/runtime';
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
type RuntimeTabKey = 'all' | 'core' | 'stack' | 'tools' | 'path';
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

interface ProgressState {
    stage: string;
    percent: number;
    downloaded: number;
    total: number;
    speedBps: number;
    detail: string;
}

interface DetailTarget {
    appId?: string;
    wingetId: string;
    name: string;
    source: string;
    version?: string;
    fallbackDesc?: string;
    isEssentialItem?: boolean;
}

let state: main.DashboardState | null = null;
let primaryTab: PrimaryTab = 'runtimes';
let runtimeTab: RuntimeTabKey = 'all';
let essentialTab: EssentialTabKey = 'all';
let installedFilter: InstalledFilterKey = 'updates';

let runtimeSearchQuery = '';
let essentialSearchQuery = '';
let wingetCatalogQuery = '';
let wingetSearchResults: essentials.WingetTableRow[] = [];
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
let showPathModal = false;
let outputDirDraft = '';

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

function detailKey(wingetId: string, source?: string): string {
    return `${wingetId.trim().toLowerCase()}|${(source || 'winget').trim().toLowerCase()}`;
}

function getCachedDetails(wingetId: string, source?: string): essentials.PackageDetails | undefined {
    return (
        wingetDetailsByKey[detailKey(wingetId, source)] ||
        wingetDetailsByKey[detailKey(wingetId, 'winget')] ||
        wingetDetailsByKey[detailKey(wingetId, 'msstore')]
    );
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

    return `
        <div class="card ${tool.installed ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(tool.id, tool.name)}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(tool.name)}</span>
                            <span class="badge badge-cat">${escapeHtml(tool.category === 'sdk' ? t.badgeSDK : t.badgeCLI)}</span>
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
                    ${renderAppIcon(item.id, item.name, { wingetId: item.wingetId, dataUrl: det?.iconDataUrl })}
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
                        data-ess="${escapeHtml(item.id)}"
                        data-name="${escapeHtml(item.name)}"
                        data-wid="${escapeHtml(item.wingetId)}"
                        data-source="${escapeHtml(item.source || 'winget')}"
                        data-ver="${escapeHtml(item.installedVer || selectedVer)}"
                        data-desc="${escapeHtml(desc)}"
                        data-isess="1"
                    >
                        ${escapeHtml(t.essDetailsBtn)}
                    </button>
                </div>
            </div>
        </div>
    `;
}

function renderWingetSearchView(): string {
    const t = getT(currentLang());
    const pinnedIDs = new Set((state?.essentials || []).map((e) => e.wingetId.toLowerCase()));
    const wingetMissing = !state?.wingetEnv?.wingetAvailable;

    let bodyHtml = `<div class="empty-state">${escapeHtml(t.essSearchEmpty)}</div>`;
    if (wingetSearching) {
        bodyHtml = `<div class="empty-state">${escapeHtml(t.essSearching)}</div>`;
    } else if (wingetSearchResults.length > 0) {
        bodyHtml = `
            <div class="cards-grid">
                ${wingetSearchResults
                    .map((row) => {
                        const isPinned = pinnedIDs.has(row.id.toLowerCase());
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

                        return `
                        <div class="card">
                            <div class="card-top">
                                <div class="card-main-row">
                                    ${renderAppIcon(row.id, row.name, { wingetId: row.id, dataUrl: det?.iconDataUrl })}
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
                            </div>
                            <div class="card-controls">
                                <div class="controls-left">
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
                                        data-name="${escapeHtml(det?.name || row.name)}"
                                        data-wid="${escapeHtml(row.id)}"
                                        data-source="${escapeHtml(row.source || 'winget')}"
                                        data-ver="${escapeHtml(row.version || '')}"
                                        data-desc="${escapeHtml(det?.description || '')}"
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
                    })
                    .join('')}
            </div>
        `;
    } else if (wingetSearchExecuted) {
        bodyHtml = `<div class="empty-state">${escapeHtml(t.essEmpty)}</div>`;
    }

    return `
        <section>
            <div class="section-header">
                <span class="section-title">${escapeHtml(t.essSearchTitle)}</span>
                <span class="section-subtitle">${escapeHtml(wingetEnriching ? t.essEnrichingSearch : t.essSearchSubtitle)}</span>
            </div>
            <div class="search-bar-row">
                <input
                    class="text-input"
                    id="winget-search-input"
                    type="text"
                    value="${escapeHtml(wingetCatalogQuery)}"
                    placeholder="${escapeHtml(t.essSearchPlaceholder)}"
                />
                <button class="btn btn-primary" id="btn-winget-search" ${wingetSearching || wingetMissing ? 'disabled' : ''}>
                    ${escapeHtml(t.essSearchBtn)}
                </button>
            </div>
            ${bodyHtml}
        </section>
    `;
}

function renderDetailModal(): string {
    if (!detailTarget) return '';
    const t = getT(currentLang());
    const det = getCachedDetails(detailTarget.wingetId, detailTarget.source);
    const curated = detailTarget.appId ? getCuratedMeta(detailTarget.appId) : undefined;
    const pinnedIDs = new Set((state?.essentials || []).map((e) => e.wingetId.toLowerCase()));
    const isPinned = pinnedIDs.has(detailTarget.wingetId.toLowerCase());
    const wingetMissing = !state?.wingetEnv?.wingetAvailable;

    const title = det?.name || detailTarget.name;
    const publisher = det?.publisher || curated?.publisher || '—';
    const homepage = det?.homepage || det?.publisherUrl || '';
    const version = det?.version || detailTarget.version || '—';
    const license = det?.license || curated?.license || '—';
    const description = det?.description || detailTarget.fallbackDesc || '';
    const iconHtml = renderAppIcon(detailTarget.appId || detailTarget.wingetId, title, {
        wingetId: detailTarget.wingetId,
        dataUrl: det?.iconDataUrl,
        sizeClass: 'lg',
    });

    const rows: string[] = [
        `<div class="detail-key">${escapeHtml(t.essPublisherLabel)}</div><div class="detail-val">${escapeHtml(publisher)}</div>`,
    ];
    if (det?.author) {
        rows.push(`<div class="detail-key">${escapeHtml(t.essAuthorLabel)}</div><div class="detail-val">${escapeHtml(det.author)}</div>`);
    }
    rows.push(`<div class="detail-key">${escapeHtml(t.essVersionLabel.replace(/:$/, ''))}</div><div class="detail-val detail-val-mono">${escapeHtml(version)}</div>`);
    rows.push(`<div class="detail-key">${escapeHtml(t.essLicenseLabel)}</div><div class="detail-val">${escapeHtml(license)}</div>`);
    const modalEssSize =
        det?.installerSize ||
        (state?.essentials || []).find((e) => e.wingetId.toLowerCase() === detailTarget!.wingetId.toLowerCase())?.downloadSize ||
        0;
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
                                <span class="badge ${detailTarget.source === 'msstore' ? 'badge-store' : 'badge-cat'}">
                                    ${escapeHtml(detailTarget.source || 'winget')}
                                </span>
                            </div>
                        </div>
                    </div>
                    <button class="btn btn-sm" id="btn-detail-close">✕</button>
                </div>

                <div class="detail-modal-scroll">
                    ${detailLoading ? `<div class="modal-note">${escapeHtml(t.essDetailsLoading)}</div>` : ''}
                    ${detailError ? `<div class="conflict-banner">${escapeHtml(detailError)}</div>` : ''}
                    <div class="detail-grid">${rows.join('')}</div>
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
                            ${busy || wingetMissing ? 'disabled' : ''}
                        >
                            ${escapeHtml(t.essInstallSilent)}
                        </button>
                        ${
                            !isPinned
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
    const existing = getCachedDetails(target.wingetId, target.source);
    if (existing && existing.loaded) {
        detailLoading = false;
        render(true);
        return;
    }

    detailLoading = true;
    render(true);

    GetWingetPackageDetails(target.wingetId, target.source)
        .then((det) => {
            wingetDetailsByKey[detailKey(target.wingetId, target.source)] = det;
            if (det.id) {
                wingetDetailsByKey[detailKey(det.id, det.source || target.source)] = det;
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
    const manageable = list.filter((a) => a.managedByWinget && (a.hasUpdate || installedFilter === 'all'));
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
    const det = getCachedDetails(app.id, app.source);
    const curated = app.curatedAppId ? getCuratedMeta(app.curatedAppId) : undefined;
    const iconID = app.curatedAppId || app.id;
    const wingetMissing = !state?.wingetEnv?.wingetAvailable;
    const dlSize = app.downloadSize || det?.installerSize || 0;

    const metaStrip = renderMetaStrip({
        publisher: det?.publisher || curated?.publisher,
        domain: shortDomain(det?.homepage || det?.publisherUrl) || curated?.domain,
        license: det?.license || curated?.license,
        installerType: det?.installerType,
    });

    const sourceBadge = app.managedByWinget
        ? `<span class="badge ${app.source === 'msstore' ? 'badge-store' : 'badge-cat'}">${escapeHtml(app.source || 'winget')}</span>`
        : `<span class="badge badge-idle" title="${escapeHtml(t.essLocalEntryNoUpdateTip)}">${escapeHtml(t.essBadgeLocalEntry)}</span>`;

    const statusBadge = app.ignored
        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeIgnored)}</span>`
        : app.hasUpdate
          ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(app.availableVer))}</span>`
          : `<span class="badge badge-active">${escapeHtml(app.installedVer ? t.essBadgeInstalled(app.installedVer) : t.statusDetected)}</span>`;

    const compareBadge = app.ignored
        ? `<span class="badge badge-ignored">${escapeHtml(t.essBadgeUpdateIgnored)}</span>`
        : app.hasUpdate
          ? `<span class="badge badge-installed">${escapeHtml(t.statusUpdateTo(app.availableVer))}</span>`
          : `<span class="badge badge-active">${escapeHtml(t.statusUpToDate)}</span>`;

    const actionsHtml = app.ignored
        ? `
            <button
                class="btn btn-sm btn-restore"
                data-action="winget-unignore"
                data-wid="${escapeHtml(app.id)}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnRestoreTip)}"
            >
                ${escapeHtml(t.essBtnRestore)}
            </button>
        `
        : `
            ${
                app.hasUpdate && app.managedByWinget
                    ? `
                    <button
                        class="btn btn-primary btn-sm"
                        data-action="winget-update-raw"
                        data-name="${escapeHtml(app.name)}"
                        data-wid="${escapeHtml(app.id)}"
                        data-source="${escapeHtml(app.source || 'winget')}"
                        ${busy || wingetMissing ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.btnUpdateTool)}
                    </button>
                `
                    : ''
            }
            <button
                class="btn btn-sm"
                data-action="winget-ignore"
                data-name="${escapeHtml(app.name)}"
                data-wid="${escapeHtml(app.id)}"
                data-source="${escapeHtml(app.source || '')}"
                ${busy ? 'disabled' : ''}
                title="${escapeHtml(t.essBtnIgnoreTip)}"
            >
                ${escapeHtml(t.essBtnIgnore)}
            </button>
            ${
                app.managedByWinget && !app.pinnedCurated
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
                    : app.pinnedCurated
                      ? `<span class="badge badge-cat">${escapeHtml(t.essAlreadyPinned)}</span>`
                      : ''
            }
        `;

    return `
        <div class="card ${!app.ignored ? 'card-active' : ''}">
            <div class="card-top">
                <div class="card-main-row">
                    ${renderAppIcon(iconID, app.name, { wingetId: app.id, dataUrl: det?.iconDataUrl })}
                    <div class="card-info">
                        <div class="card-title-row">
                            <span class="card-name">${escapeHtml(det?.name || app.name)}</span>
                            <span class="card-id">[${escapeHtml(app.id)}]</span>
                            ${sourceBadge}
                            ${app.ignored && app.wingetPinned ? `<span class="badge badge-cat">${escapeHtml(t.essBadgeWingetPinned)}</span>` : ''}
                        </div>
                        ${det?.description ? `<div class="card-desc card-desc-clamp">${escapeHtml(det.description)}</div>` : ''}
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
                                <span class="vc-label">${escapeHtml(t.essWingetLatestLabel)}</span>
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
                        app.managedByWinget
                            ? `
                            <button
                                class="btn btn-sm"
                                data-action="ess-detail"
                                data-ess="${escapeHtml(app.curatedAppId || '')}"
                                data-name="${escapeHtml(det?.name || app.name)}"
                                data-wid="${escapeHtml(app.id)}"
                                data-source="${escapeHtml(app.source || 'winget')}"
                                data-ver="${escapeHtml(app.availableVer || app.installedVer || '')}"
                                data-desc="${escapeHtml(det?.description || '')}"
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
    const ignoredList = allRows.filter((a) => a.ignored);

    const baseList =
        installedFilter === 'updates' ? activeUpdates : installedFilter === 'ignored' ? ignoredList : activeInstalled;
    const filtered = baseList.filter((a) =>
        matchesSearch(`${a.name} ${a.id} ${a.installedVer} ${a.availableVer} ${a.source}`, essentialSearchQuery),
    );

    const updatableCount = activeUpdates.filter((a) => a.managedByWinget).length;
    const wingetMissing = !s.wingetEnv?.wingetAvailable;
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
                        ${busy || wingetMissing || updatableCount === 0 ? 'disabled' : ''}
                    >
                        ${escapeHtml(t.essUpdateAll(updatableCount))}
                    </button>
                </div>
            </div>

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
    const baseLower = s.baseDir.toLowerCase();
    const rows = s.userPathList
        .map((entry, idx) => {
            const managed = entry.toLowerCase().startsWith(baseLower);
            return `
                <div class="path-entry-row ${managed ? 'managed' : ''}">
                    <span class="path-index">#${idx + 1}</span>
                    <span>${escapeHtml(entry)}</span>
                    ${managed ? `<span class="badge badge-active" style="margin-left:auto">${escapeHtml(t.managedBadge)}</span>` : ''}
                </div>
            `;
        })
        .join('');

    return `
        <div>
            <div class="section-header">
                <span class="section-title">${escapeHtml(t.pathInspectorTitle)}</span>
                <span class="section-subtitle">${escapeHtml(t.pathInspectorSubtitle(s.userPathList.length))}</span>
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
    const cliTools = s.tools.filter((tl) =>
        matchesSearch(`${tl.name} ${tl.id} ${tl.description} ${tl.commandHint}`, runtimeSearchQuery),
    );
    const activeApps = s.packages.filter((p) => Boolean(p.activeVer));
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
                <span class="nav-count">${s.tools.length}</span>
            </button>
            <button class="nav-item ${runtimeTab === 'path' ? 'active' : ''}" data-rt-tab="path">
                <span>${escapeHtml(t.navUserPath)}</span>
                <span class="nav-count">${s.userPathList.length}</span>
            </button>
        </div>

        <div class="active-summary">
            <div class="nav-section-title">${escapeHtml(t.activeInPathTitle)} (${activeApps.length})</div>
            ${
                activeApps.length > 0
                    ? activeApps
                          .map(
                              (a) => `
                        <div class="active-chip">
                            <span class="active-chip-name">${escapeHtml(a.id)}</span>
                            <span class="active-chip-ver">v${escapeHtml(a.activeVer)}</span>
                        </div>
                    `,
                          )
                          .join('')
                    : `<div class="card-desc" style="padding:0 6px;font-size:11.5px">${escapeHtml(t.noActiveInPath)}</div>`
            }
        </div>
    `;

    return { sidebarHtml, contentHtml };
}

function renderEssentialsView(s: main.DashboardState): { sidebarHtml: string; contentHtml: string } {
    const t = getT(currentLang());
    const allEss = s.essentials || [];
    const selectedCount = allEss.filter((e) => Boolean(checkedEssentials[e.id])).length;

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
                    <span class="section-title">${escapeHtml(t.tabCleanInstall)} (${filtered.length})</span>
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
            <div class="nav-section-title">${escapeHtml(t.essBatchTitle)}</div>
            <div class="card-desc" style="padding:0 4px">${escapeHtml(t.essSelectedCount(selectedCount))}</div>
            <div class="controls-left" style="gap:6px">
                <button class="btn btn-sm" id="btn-ess-select-missing" ${busy ? 'disabled' : ''}>${escapeHtml(t.essSelectMissing)}</button>
                <button class="btn btn-sm" id="btn-ess-clear-sel" ${busy || selectedCount === 0 ? 'disabled' : ''}>${escapeHtml(t.essClearSel)}</button>
            </div>
            <button
                class="btn btn-primary btn-sm"
                id="btn-ess-install-batch"
                style="justify-content:center;margin-top:4px"
                ${busy || selectedCount === 0 || !s.wingetEnv.wingetAvailable ? 'disabled' : ''}
            >
                ${escapeHtml(t.essInstallSelected(selectedCount))}
            </button>
        </div>
    `;

    return { sidebarHtml, contentHtml };
}

function render(skipScrollCapture = false): void {
    const lang = currentLang();
    const t = getT(lang);

    if (!state) {
        appRoot.innerHTML = `<div class="empty-state" style="margin:auto">${escapeHtml(t.loading)}</div>`;
        return;
    }

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

    const topbarLeftMeta =
        primaryTab === 'runtimes'
            ? `
                <button class="pill pill-btn" id="btn-output-modal" ${busy ? 'disabled' : ''} title="${escapeHtml(t.outputChangeTooltip)}">
                    <span>${escapeHtml(t.outputLabel)}: ${escapeHtml(state.baseDir)}</span>
                    <span style="color: var(--accent-blue)">✎</span>
                </button>
                <span class="pill pill-accent">HKCU User PATH</span>
                ${state.lastLtsCheck ? `<span class="pill">${escapeHtml(t.checkedAt(state.lastLtsCheck))}</span>` : ''}
            `
            : `
                <span class="pill ${state.wingetEnv.wingetAvailable ? 'pill-accent' : ''}">
                    ${escapeHtml(state.wingetEnv.wingetAvailable ? `Winget ${state.wingetEnv.wingetVersion}` : 'Winget Missing')}
                </span>
                <span class="pill">${escapeHtml(state.wingetEnv.isAdmin ? 'Admin' : 'Standard User')}</span>
            `;

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
                <span class="brand-title">DevApps</span>
                <div class="mode-tabs">
                    <button class="mode-tab ${primaryTab === 'runtimes' ? 'active' : ''}" data-primary-tab="runtimes">${escapeHtml(t.tabRuntimes)}</button>
                    <button class="mode-tab ${primaryTab === 'essentials' ? 'active' : ''}" data-primary-tab="essentials">${escapeHtml(t.tabCleanInstall)}</button>
                </div>
                ${topbarLeftMeta}
            </div>
            <div class="topbar-actions">
                <div class="lang-switch" title="Language / Bahasa">
                    <button class="lang-btn ${lang === 'en' ? 'active' : ''}" data-lang="en">EN</button>
                    <button class="lang-btn ${lang === 'id' ? 'active' : ''}" data-lang="id">ID</button>
                </div>
                ${topbarRightActions}
            </div>
        </header>

        <div class="workspace">
            <aside class="sidebar">
                ${sidebarHtml}
            </aside>
            <main class="content">
                ${contentHtml}
            </main>
        </div>

        <section class="terminal-drawer ${terminalCollapsed ? 'collapsed' : ''}">
            <div class="terminal-header">
                <div class="terminal-title-group">
                    <span class="terminal-label">${escapeHtml(t.terminalTitle)}</span>
                    <div id="progress-slot">${renderProgressInline()}</div>
                </div>
                <div class="terminal-actions">
                    ${busy ? `<button class="btn btn-danger btn-sm" id="btn-cancel">${escapeHtml(t.btnCancel)}</button>` : ''}
                    <button class="btn btn-sm" id="btn-clear-log">${escapeHtml(t.btnClear)}</button>
                    <button class="btn btn-sm" id="btn-toggle-term">${escapeHtml(terminalCollapsed ? t.btnExpand : t.btnMinimize)}</button>
                </div>
            </div>
            <div class="terminal-body" id="terminal-body"></div>
        </section>

        ${
            showPathModal
                ? `
            <div class="modal-backdrop" id="output-modal-backdrop">
                <div class="modal-card">
                    <div class="modal-header">
                        <span class="modal-title">${escapeHtml(t.modalTitle)}</span>
                        <button class="btn btn-sm" id="btn-modal-close">✕</button>
                    </div>
                    <div class="card-desc">${escapeHtml(t.modalDesc)}</div>
                    <div class="modal-row">
                        <input class="text-input" id="output-dir-input" type="text" value="${escapeHtml(outputDirDraft || state.baseDir)}" placeholder="${escapeHtml(t.modalPlaceholder)}" />
                        <button class="btn" id="btn-modal-browse">${escapeHtml(t.modalBrowse)}</button>
                    </div>
                    <div class="modal-note">
                        <div><strong>${escapeHtml(t.modalActiveConfig)}</strong> <code>${escapeHtml(state.confPath)}</code> &amp; <code>${escapeHtml(state.essentialsConfPath)}</code></div>
                        <div>${escapeHtml(t.modalCopyNote)}</div>
                    </div>
                    <div class="modal-footer">
                        <div class="controls-left">
                            <button class="btn btn-sm" id="btn-modal-sync" title="${escapeHtml(t.modalSyncTooltip)}">${escapeHtml(t.modalSyncDefaults)}</button>
                            <button class="btn btn-sm" id="btn-modal-reset">${escapeHtml(t.modalReset)}</button>
                        </div>
                        <div class="controls-right">
                            <button class="btn btn-sm" id="btn-modal-cancel">${escapeHtml(t.modalCancel)}</button>
                            <button class="btn btn-primary btn-sm" id="btn-modal-save">${escapeHtml(t.modalSave)}</button>
                        </div>
                    </div>
                </div>
            </div>
        `
                : ''
        }
        ${renderDetailModal()}
    `;

    renderTerminalLinesOnly();
    bindEventListeners();
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

function runWingetCatalogSearch(queryOverride?: string): void {
    const q = (queryOverride ?? wingetCatalogQuery).trim();
    if (!q) return;
    wingetCatalogQuery = q;
    wingetSearching = true;
    wingetEnriching = false;
    wingetSearchExecuted = true;
    const token = ++wingetSearchToken;
    render();
    SearchWingetApps(q)
        .then((rows) => {
            if (token !== wingetSearchToken) return;
            const list = rows || [];
            wingetSearchResults = list;
            wingetSearching = false;
            if (list.length > 0) {
                wingetEnriching = true;
                render();
                EnrichWingetSearch(list)
                    .then((detailsList) => {
                        if (token !== wingetSearchToken) return;
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
                        // Search results still usable without enrichment.
                    })
                    .finally(() => {
                        if (token === wingetSearchToken) {
                            wingetEnriching = false;
                            render(true);
                        }
                    });
            } else {
                render();
            }
        })
        .catch((e) => {
            if (token !== wingetSearchToken) return;
            wingetSearchResults = [];
            wingetSearching = false;
            wingetEnriching = false;
            reportErr(e);
            render();
        });
}

function bindEventListeners(): void {
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

    document.getElementById('btn-output-modal')?.addEventListener('click', () => {
        outputDirDraft = state?.baseDir || '';
        showPathModal = true;
        render();
    });

    document.getElementById('btn-modal-close')?.addEventListener('click', () => {
        showPathModal = false;
        render();
    });

    document.getElementById('btn-modal-cancel')?.addEventListener('click', () => {
        showPathModal = false;
        render();
    });

    document.getElementById('output-modal-backdrop')?.addEventListener('click', (ev) => {
        if (ev.target === ev.currentTarget) {
            showPathModal = false;
            render();
        }
    });

    const dirInputEl = document.getElementById('output-dir-input') as HTMLInputElement | null;
    dirInputEl?.addEventListener('input', (ev) => {
        outputDirDraft = (ev.target as HTMLInputElement).value;
    });
    dirInputEl?.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter') {
            document.getElementById('btn-modal-save')?.click();
        } else if (ev.key === 'Escape') {
            showPathModal = false;
            render();
        }
    });

    document.getElementById('btn-modal-browse')?.addEventListener('click', () => {
        PickOutputDirectory()
            .then((picked) => {
                if (picked) {
                    outputDirDraft = picked;
                    render();
                }
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-modal-reset')?.addEventListener('click', () => {
        outputDirDraft = 'C:\\AppPath';
        render();
    });

    document.getElementById('btn-modal-sync')?.addEventListener('click', () => {
        SyncDefaultPackages()
            .then((nextState) => {
                state = nextState;
                showPathModal = false;
                render();
            })
            .catch((e) => reportErr(e));
    });

    document.getElementById('btn-modal-save')?.addEventListener('click', () => {
        const target = (document.getElementById('output-dir-input') as HTMLInputElement | null)?.value || outputDirDraft;
        SetOutputDir(target)
            .then((nextState) => {
                state = nextState;
                showPathModal = false;
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

    document.getElementById('btn-ignored-conf')?.addEventListener('click', () => {
        OpenIgnoredConf().catch((e) => reportErr(e));
    });

    document.getElementById('btn-update-all-installed')?.addEventListener('click', () => {
        const targets = (state?.installedApps || [])
            .filter((a) => !a.ignored && a.hasUpdate && a.managedByWinget)
            .map((a) =>
                main.WingetUpdateTarget.createFrom({
                    name: a.name,
                    wingetId: a.id,
                    source: a.source || 'winget',
                }),
            );
        if (targets.length > 0) {
            UpdateAllInstalled(targets).catch((e) => reportErr(e));
        }
    });

    document.getElementById('btn-ess-select-missing')?.addEventListener('click', () => {
        for (const item of state?.essentials || []) {
            if (!item.installed) {
                checkedEssentials[item.id] = true;
            }
        }
        render();
    });

    document.getElementById('btn-ess-clear-sel')?.addEventListener('click', () => {
        checkedEssentials = {};
        render();
    });

    document.getElementById('btn-ess-install-batch')?.addEventListener('click', () => {
        triggerBatchInstall();
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
            } else if (action === 'ess-run') {
                const essID = btn.dataset.ess || '';
                const mode = btn.dataset.mode || 'install';
                const essVer = essID ? selectedEssentialVerByApp[essID] || 'latest' : 'latest';
                if (essID) {
                    RunEssentialTool(essID, mode, essVer).catch((e) => reportErr(e));
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
            } else if (action === 'winget-ignore') {
                const name = btn.dataset.name || '';
                const wid = btn.dataset.wid || '';
                const source = btn.dataset.source || '';
                if (wid) {
                    IgnoreInstalledApp(wid, name, source, '')
                        .then((nextState) => {
                            state = nextState;
                            render();
                        })
                        .catch((e) => reportErr(e));
                }
            } else if (action === 'winget-unignore') {
                const wid = btn.dataset.wid || '';
                if (wid) {
                    UnignoreInstalledApp(wid)
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
                        appId: btn.dataset.ess || undefined,
                        wingetId: wid,
                        name: btn.dataset.name || wid,
                        source: btn.dataset.source || 'winget',
                        version: btn.dataset.ver || undefined,
                        fallbackDesc: btn.dataset.desc || undefined,
                        isEssentialItem: btn.dataset.isess === '1',
                    });
                }
            }
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
        detailTarget = null;
        if (target.isEssentialItem && target.appId) {
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

window.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && detailTarget) {
        detailTarget = null;
        render(true);
    }
});

void refreshState();
