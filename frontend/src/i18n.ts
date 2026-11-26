export type Lang = 'en' | 'id';

export interface Translations {
    loading: string;
    tabRuntimes: string;
    tabWindowsApps: string;
    outputLabel: string;
    outputChangeTooltip: string;
    btnPackages: string;
    btnPackagesTooltip: string;
    btnEssentialsConf: string;
    btnEssentialsConfTooltip: string;
    btnCheckUpdates: string;
    btnCheckUpdatesTooltip: string;
    btnReload: string;
    btnReloadTooltip: string;
    btnOpenFolder: string;
    btnOpenFolderTooltip: string;
    btnRefreshWinget: string;
    searchPlaceholder: string;
    searchEssPlaceholder: string;
    navWorkspace: string;
    navAll: string;
    navCore: string;
    navStack: string;
    navTools: string;
    navUserPath: string;
    activeInPathTitle: string;
    noActiveInPath: string;
    secCoreTitle: string;
    secCoreSubtitle: (baseDir: string) => string;
    secStackTitle: string;
    secStackSubtitle: string;
    secToolsTitle: string;
    secToolsSubtitle: string;
    emptyCore: string;
    emptyStack: string;
    emptyTools: string;
    badgeCore: string;
    badgeStack: string;
    badgeCLI: string;
    badgeSDK: string;
    badgeNotInstalled: string;
    badgeExtractedOnly: string;
    badgeActiveInPath: (ver: string) => string;
    pathConflict: (scope: string, fullPath: string) => string;
    noVersions: string;
    btnDownload: string;
    btnReDownload: string;
    btnDownloadTooltip: (baseDir: string) => string;
    btnNoUrlTooltip: string;
    downloadSizeLabel: (size: string) => string;
    toolSizeWebView2Tip: string;
    toolSizeVSBuild: (bootSize: string) => string;
    toolSizeVSBuildTip: string;
    toolSizeGoDynamic: string;
    toolSizeGoDynamicTip: string;
    toolSizeCargoHeavy: string;
    toolSizeCargoHeavyTip: string;
    btnAddToPath: string;
    btnSwitchPath: string;
    btnActiveInPath: string;
    tipDownloadFirst: string;
    tipAlreadyActive: string;
    tipAddToPath: string;
    btnRemovePath: string;
    tipRemovePath: string;
    btnDelete: string;
    tipDelete: string;
    btnInstallCmd: string;
    btnUpdateTool: string;
    btnFixTool: string;
    tipUpdateAvailable: (ver: string) => string;
    tipNoUpdate: string;
    tipCheckingUpdate: string;
    tipFixTool: string;
    btnAddDirToPath: (dir: string) => string;
    locationLabel: string;
    requiresLabel: string;
    activeOnWindows: string;
    installedLabel: string;
    notInPath: string;
    latestLabel: string;
    latestBranchLabel: (branch: string) => string;
    latestMainLabel: string;
    checkingLabel: string;
    statusUpToDate: string;
    statusUpToDateBranch: (branch: string) => string;
    statusUpdateTo: (ver: string) => string;
    statusUpdateBranch: (branch: string, ver: string) => string;
    statusAhead: string;
    statusDetected: string;
    pathInspectorTitle: string;
    pathInspectorSubtitle: (count: number) => string;
    pathInspectorEmpty: string;
    managedBadge: string;
    pathBtnAddFolder: string;
    pathBtnClean: (n: number) => string;
    pathBtnRefresh: string;
    pathBtnMoveUpTip: string;
    pathBtnMoveDownTip: string;
    pathBtnOpen: string;
    pathBtnOpenTip: string;
    pathBtnRemove: string;
    pathBtnRemoveTip: string;
    pathBadgeMissing: string;
    pathBadgeDuplicate: string;
    terminalTitle: string;
    btnCancel: string;
    btnClear: string;
    btnMinimize: string;
    btnExpand: string;
    progressDownloading: string;
    progressInstalling: string;
    progressProcessing: string;
    modalTitle: string;
    modalDesc: string;
    modalPlaceholder: string;
    modalBrowse: string;
    modalActiveConfig: string;
    modalCopyNote: string;
    modalSyncDefaults: string;
    modalSyncTooltip: string;
    modalReset: string;
    modalCancel: string;
    modalSave: string;
    checkedAt: (ts: string) => string;

    // Windows Apps
    essCatAll: string;
    essCatBrowsers: string;
    essCatUtilities: string;
    essCatEditors: string;
    essCatOffice: string;
    essCatChat: string;
    essCatDownloaders: string;
    essCatSearch: string;
    essBatchTitle: string;
    essSelectedCount: (n: number) => string;
    essSelectMissing: string;
    essClearSel: string;
    essInstallSelected: (n: number) => string;
    essWingetReady: (ver: string) => string;
    essWingetMissing: string;
    essInstallWinget: string;
    essUacAdmin: string;
    essUacStandard: string;
    essRestartAdmin: string;
    essConfigStoredHint: (path: string) => string;
    essInstallSilent: string;
    essReinstallSilent: string;
    essBadgeInstalled: (ver: string) => string;
    essBadgeInstalledNoVer: string;
    essStatusNewerThanWinget: string;
    essWingetLatestLabel: string;
    essBadgeMsStore: string;
    essBadgeUAC: string;
    essSecSubtitle: string;
    essEmpty: string;
    essSearchInWinget: (q: string) => string;
    essSearchTitle: string;
    essSearchSubtitle: string;
    essSearchPlaceholder: string;
    essSearchBtn: string;
    essSearchEmpty: string;
    essSearching: string;
    essPinToList: string;
    essAlreadyPinned: string;
    essVersionLabel: string;
    essLaragonNoPopupNote: string;
    essDetailsBtn: string;
    essDetailsLoading: string;
    essEnrichingSearch: string;
    essPublisherLabel: string;
    essAuthorLabel: string;
    essHomepageLabel: string;
    essLicenseLabel: string;
    essInstallerSizeLabel: string;
    essInstallerTypeLabel: string;
    essInstallerUrlLabel: string;
    essSha256Label: string;
    essReleaseDateLabel: string;
    essTagsLabel: string;
    essDescriptionLabel: string;
    essOpenHomepage: string;
    essModalClose: string;
    essDetailsNoData: string;
    btnIgnoredConf: string;
    btnIgnoredConfTooltip: string;
    essCatInstalled: string;
    essInstalledTitle: string;
    essInstalledSubtitle: string;
    essFilterUpdates: (n: number) => string;
    essFilterAllInstalled: (n: number) => string;
    essFilterIgnored: (n: number) => string;
    essUpdateAll: (n: number) => string;
    essUpdatesEmpty: string;
    essInstalledEmpty: string;
    essIgnoredEmpty: string;
    essBtnIgnore: string;
    essBtnIgnoreTip: string;
    essBtnRestore: string;
    essBtnRestoreTip: string;
    essBadgeIgnored: string;
    essBadgeUpdateIgnored: string;
    essBadgeWingetPinned: string;
    essBadgeLocalEntry: string;
    essLocalEntryNoUpdateTip: string;

    // About Modal
    btnAbout: string;
    btnAboutTooltip: string;
    aboutTitle: string;
    aboutSubtitle: string;
    aboutDesc: string;
    aboutVersionLabel: string;
    aboutAuthorLabel: string;
    aboutEmailLabel: string;
    aboutWebsiteLabel: string;
    aboutTelegramLabel: string;
    aboutRepoLabel: string;
    aboutLicenseLabel: string;
    aboutTechLabel: string;
    aboutAppDirLabel: string;
    aboutOutputDirLabel: string;
    aboutOpenRepo: string;
    aboutOpenTelegram: string;
    aboutUpdateAvailableBadge: (ver: string) => string;
    aboutUpdateAvailableTip: (ver: string) => string;
    aboutUpdateTitle: string;
    aboutUpdateChecking: string;
    aboutUpdateUpToDate: (ver: string) => string;
    aboutUpdateAvailableMsg: (cur: string, latest: string) => string;
    aboutUpdateCheckBtn: string;
    aboutUpdateInstallBtn: string;
    aboutUpdateOpenReleaseBtn: string;
    aboutUpdateNoAssetNote: string;
    aboutUpdateNotesLabel: string;
    aboutUpdateLastChecked: (ts: string) => string;
}

const en: Translations = {
    loading: 'Loading UniDevUI...',
    tabRuntimes: 'Portable Runtimes',
    tabWindowsApps: 'Windows Apps',
    outputLabel: 'Output',
    outputChangeTooltip: 'Click to change archive extraction output directory',
    btnPackages: 'Packages (.conf)',
    btnPackagesTooltip: 'Open packages.conf in Notepad (stored next to UniDevUI.exe)',
    btnEssentialsConf: 'Essentials (.conf)',
    btnEssentialsConfTooltip: 'Open essentials.conf in Notepad (stored next to UniDevUI.exe)',
    btnCheckUpdates: 'Check Updates',
    btnCheckUpdatesTooltip: 'Check and update latest LTS/stable links, CLI tools & Winget status',
    btnReload: 'Reload',
    btnReloadTooltip: 'Reload packages.conf, essentials.conf & disk/Winget status',
    btnOpenFolder: 'Open Folder',
    btnOpenFolderTooltip: 'Open extraction output directory in Explorer',
    btnRefreshWinget: 'Refresh Winget',
    searchPlaceholder: 'Filter apps or tools...',
    searchEssPlaceholder: 'Filter Windows apps...',
    navWorkspace: 'Workspace',
    navAll: 'All Packages',
    navCore: 'Core Runtimes',
    navStack: 'Dev Stack',
    navTools: 'CLI & SDK Tools',
    navUserPath: 'User PATH',
    activeInPathTitle: 'Active in PATH',
    noActiveInPath: 'No UniDevUI tools in User PATH yet.',
    secCoreTitle: 'Core Runtimes & Portable Apps',
    secCoreSubtitle: (baseDir) => `Extracted to ${baseDir} · Activate PATH manually per app`,
    secStackTitle: 'Dev Stack & Compilers (Portable Archives)',
    secStackSubtitle: '.NET SDK LTS, Rust, WinLibs GCC, CMake, 7-Zip CLI',
    secToolsTitle: 'CLI & Windows SDK Installers (via CMD / Winget)',
    secToolsSubtitle: 'Wails CLI, Tauri CLI, VS Build Tools 2022 & WebView2 Runtime',
    emptyCore: 'No matching core packages.',
    emptyStack: 'No matching dev stack packages.',
    emptyTools: 'No matching CLI or SDK tools.',
    badgeCore: 'Core',
    badgeStack: 'Dev Stack',
    badgeCLI: 'CLI Tool',
    badgeSDK: 'Windows SDK / Runtime',
    badgeNotInstalled: 'Not Installed',
    badgeExtractedOnly: 'Extracted (Not in PATH)',
    badgeActiveInPath: (ver) => `Active in PATH · v${ver}`,
    pathConflict: (scope, fullPath) => `PATH Conflict in ${scope}: ${fullPath} takes higher priority in Windows.`,
    noVersions: '(No versions)',
    btnDownload: 'Download & Extract',
    btnReDownload: 'Re-Download',
    btnDownloadTooltip: (baseDir) => `Download archive & extract to ${baseDir}`,
    btnNoUrlTooltip: 'Download URL missing in packages.conf',
    downloadSizeLabel: (size) => `Download archive size: ${size}`,
    toolSizeWebView2Tip: 'Official standalone x64 Edge WebView2 installer size (via Winget)',
    toolSizeVSBuild: (bootSize) =>
        bootSize ? `Bootstrapper ${bootSize} (+ multi-GB VCTools)` : 'Bootstrapper + multi-GB VCTools',
    toolSizeVSBuildTip: 'vs_BuildTools.exe bootstrapper is ~4.3 MB, then downloads several GB of MSVC & Windows SDK components',
    toolSizeGoDynamic: 'Dynamic (go install)',
    toolSizeGoDynamicTip: 'Downloads Go modules dynamically & compiles locally (size depends on your Go module cache)',
    toolSizeCargoHeavy: 'Heavy compile (cargo install)',
    toolSizeCargoHeavyTip: 'Downloads many Rust crates & compiles locally (hundreds of MB / several minutes)',
    btnAddToPath: 'Add to PATH',
    btnSwitchPath: 'Switch PATH',
    btnActiveInPath: 'Active in PATH',
    tipDownloadFirst: 'Download & Extract this version first before adding to User PATH',
    tipAlreadyActive: 'This version is already active in User PATH',
    tipAddToPath: 'Add or replace this app entry in User PATH (HKCU)',
    btnRemovePath: 'Remove PATH',
    tipRemovePath: 'Remove this app from User PATH',
    btnDelete: 'Delete',
    tipDelete: 'Delete extracted folder for this version',
    btnInstallCmd: 'Install via CMD',
    btnUpdateTool: 'Update',
    btnFixTool: 'Fix',
    tipUpdateAvailable: (ver) => `Update to v${ver}`,
    tipNoUpdate: 'Already on the latest version (no update available)',
    tipCheckingUpdate: 'Checking latest version...',
    tipFixTool: 'Repair or reinstall this tool if broken',
    btnAddDirToPath: (dir) => `Add ${dir} to PATH`,
    locationLabel: 'Location',
    requiresLabel: 'Requires',
    activeOnWindows: 'Active on Windows:',
    installedLabel: 'Installed:',
    notInPath: 'Not in PATH',
    latestLabel: 'Latest:',
    latestBranchLabel: (branch) => `Latest v${branch}.x:`,
    latestMainLabel: 'Latest Main:',
    checkingLabel: 'Checking...',
    statusUpToDate: '✓ Up to date',
    statusUpToDateBranch: (branch) => `✓ v${branch}.x Latest`,
    statusUpdateTo: (ver) => `↑ Update available (v${ver})`,
    statusUpdateBranch: (branch, ver) => `↑ Update v${branch}.x → v${ver}`,
    statusAhead: '★ Newer than .conf',
    statusDetected: 'Detected',
    pathInspectorTitle: 'User PATH Inspector (HKCU\\Environment\\Path)',
    pathInspectorSubtitle: (count) => `${count} entries · Evaluated from top to bottom`,
    pathInspectorEmpty: 'User PATH (HKCU\\Environment) is currently empty.',
    managedBadge: 'Managed by UniDevUI',
    pathBtnAddFolder: '+ Add Folder',
    pathBtnClean: (n) => `Clean Missing/Dup (${n})`,
    pathBtnRefresh: 'Refresh',
    pathBtnMoveUpTip: 'Move up (higher PATH priority)',
    pathBtnMoveDownTip: 'Move down (lower PATH priority)',
    pathBtnOpen: 'Open',
    pathBtnOpenTip: 'Open directory in Explorer',
    pathBtnRemove: 'Remove',
    pathBtnRemoveTip: 'Remove this entry from User PATH',
    pathBadgeMissing: 'Missing',
    pathBadgeDuplicate: 'Duplicate',
    terminalTitle: 'TERMINAL OUTPUT',
    btnCancel: 'Cancel',
    btnClear: 'Clear',
    btnMinimize: 'Minimize',
    btnExpand: 'Expand',
    progressDownloading: 'Downloading...',
    progressInstalling: 'Running installer command...',
    progressProcessing: 'Processing...',
    modalTitle: 'Output Path & Configuration Settings',
    modalDesc: 'Choose where downloaded archives are extracted (config files stay beside UniDevUI.exe):',
    modalPlaceholder: 'Example: D:\\DevTools',
    modalBrowse: 'Browse...',
    modalActiveConfig: 'Configs (beside UniDevUI.exe):',
    modalCopyNote: 'Changing output path only redirects archive extraction. packages.conf and essentials.conf always stay in the UniDevUI.exe folder.',
    modalSyncDefaults: 'Sync Default Apps',
    modalSyncTooltip: 'Append missing built-in apps to packages.conf and essentials.conf without overwriting your edits',
    modalReset: 'Reset C:\\AppPath',
    modalCancel: 'Cancel',
    modalSave: 'Apply Path',
    checkedAt: (ts) => `Checked: ${ts}`,

    essCatAll: 'All Essentials',
    essCatBrowsers: 'Browsers',
    essCatUtilities: 'Archives & Utilities',
    essCatEditors: 'Editors & Shells',
    essCatOffice: 'Media & Office',
    essCatChat: 'Chat, Mail & Cloud',
    essCatDownloaders: 'Download Managers',
    essCatSearch: 'Winget Search',
    essBatchTitle: 'Batch Install Queue',
    essSelectedCount: (n) => `${n} app(s) selected`,
    essSelectMissing: 'Select Missing',
    essClearSel: 'Clear',
    essInstallSelected: (n) => `Install Selected (${n})`,
    essWingetReady: (ver) => `Winget Ready (${ver || 'detected'})`,
    essWingetMissing: 'Winget not found on this Windows system',
    essInstallWinget: 'Install Winget',
    essUacAdmin: 'UAC: Administrator',
    essUacStandard: 'UAC: Standard User (some installers may prompt UAC)',
    essRestartAdmin: 'Restart as Admin',
    essConfigStoredHint: (path) => `Config: ${path}`,
    essInstallSilent: 'Install (Silent)',
    essReinstallSilent: 'Reinstall',
    essBadgeInstalled: (ver) => `Installed · v${ver}`,
    essBadgeInstalledNoVer: 'Installed',
    essStatusNewerThanWinget: '★ Newer than Winget',
    essWingetLatestLabel: 'Winget:',
    essBadgeMsStore: 'MS Store',
    essBadgeUAC: 'UAC / Admin',
    essSecSubtitle: 'Silent install Windows apps via Winget · Check boxes to install multiple apps at once',
    essEmpty: 'No matching Windows apps.',
    essSearchInWinget: (q) => `Search "${q}" in Winget Catalog →`,
    essSearchTitle: 'Winget Catalog Search',
    essSearchSubtitle: 'Search any Windows application outside the built-in 27 list and install silently',
    essSearchPlaceholder: 'Type app name or Winget ID (e.g. obsidian, everything, 7zip)...',
    essSearchBtn: 'Search Winget',
    essSearchEmpty: 'Enter a keyword above to search the official Winget & MS Store catalog.',
    essSearching: 'Searching Winget catalog...',
    essPinToList: '+ Add to List',
    essAlreadyPinned: 'In List',
    essVersionLabel: 'Version:',
    essLaragonNoPopupNote: '6.0.0 (no license popup)',
    essDetailsBtn: 'Details',
    essDetailsLoading: 'Fetching live Winget metadata & icon...',
    essEnrichingSearch: 'Loading publishers, descriptions & icons...',
    essPublisherLabel: 'Publisher',
    essAuthorLabel: 'Author',
    essHomepageLabel: 'Homepage',
    essLicenseLabel: 'License',
    essInstallerSizeLabel: 'Download Size',
    essInstallerTypeLabel: 'Installer',
    essInstallerUrlLabel: 'Installer URL',
    essSha256Label: 'SHA256',
    essReleaseDateLabel: 'Release Date',
    essTagsLabel: 'Tags',
    essDescriptionLabel: 'Description',
    essOpenHomepage: 'Open Homepage',
    essModalClose: 'Close',
    essDetailsNoData: 'No additional Winget metadata returned for this package.',
    btnIgnoredConf: 'Ignored (.conf)',
    btnIgnoredConfTooltip: 'Open ignored_updates.conf in Notepad (stored next to UniDevUI.exe)',
    essCatInstalled: 'Installed & Updates',
    essInstalledTitle: 'Installed Apps & Updates',
    essInstalledSubtitle: 'Scans installed Windows apps via Winget · Update selectively or ignore apps you keep pinned',
    essFilterUpdates: (n) => `Updates Available (${n})`,
    essFilterAllInstalled: (n) => `All Installed (${n})`,
    essFilterIgnored: (n) => `Ignored (${n})`,
    essUpdateAll: (n) => `Update All (${n})`,
    essUpdatesEmpty: 'All non-ignored apps are up to date!',
    essInstalledEmpty: 'No matching installed apps found. Click Refresh Winget to rescan.',
    essIgnoredEmpty: 'No ignored apps yet. Click Ignore on any app (like Laragon v6) to exclude it from updates.',
    essBtnIgnore: 'Ignore',
    essBtnIgnoreTip: 'Hide from Updates & All Installed, and set a blocking pin in Windows Winget if supported',
    essBtnRestore: 'Restore',
    essBtnRestoreTip: 'Restore to Installed & Updates tracking and remove any Windows Winget pin',
    essBadgeIgnored: 'Ignored',
    essBadgeUpdateIgnored: 'Update ignored',
    essBadgeWingetPinned: 'Winget Pinned',
    essBadgeLocalEntry: 'Local Entry',
    essLocalEntryNoUpdateTip: 'Local ARP/MSIX entry has no Winget catalog ID; ignored locally in UniDevUI',

    btnAbout: 'About',
    btnAboutTooltip: 'About UniDevUI, version, developer & license',
    aboutTitle: 'About UniDevUI',
    aboutSubtitle: 'Portable Runtimes & Windows Apps Manager',
    aboutDesc:
        'Portable Windows desktop utility for setting up developer runtimes cleanly without polluting the OS (safe HKCU User PATH management) and automating silent Windows app installs, updates, and Winget version pins.',
    aboutVersionLabel: 'Version',
    aboutAuthorLabel: 'Developer',
    aboutEmailLabel: 'Email',
    aboutWebsiteLabel: 'Website',
    aboutTelegramLabel: 'Telegram',
    aboutRepoLabel: 'GitHub Repo',
    aboutLicenseLabel: 'License',
    aboutTechLabel: 'Built With',
    aboutAppDirLabel: 'App Folder',
    aboutOutputDirLabel: 'Output Folder',
    aboutOpenRepo: 'Open GitHub',
    aboutOpenTelegram: 'Contact Telegram',
    aboutUpdateAvailableBadge: (ver) => `v${ver}`,
    aboutUpdateAvailableTip: (ver) => `UniDevUI v${ver} is available! Click to view & update.`,
    aboutUpdateTitle: 'Application Update',
    aboutUpdateChecking: 'Checking GitHub Releases for updates...',
    aboutUpdateUpToDate: (ver) => `UniDevUI v${ver} is up to date`,
    aboutUpdateAvailableMsg: (cur, latest) => `New release available: v${cur} → v${latest}`,
    aboutUpdateCheckBtn: 'Check App Update',
    aboutUpdateInstallBtn: 'Update & Restart',
    aboutUpdateOpenReleaseBtn: 'Release Page',
    aboutUpdateNoAssetNote: 'Installer asset not found in release; please download from Release Page.',
    aboutUpdateNotesLabel: 'Release Notes',
    aboutUpdateLastChecked: (ts) => `Last checked: ${ts}`,
};

const id: Translations = {
    loading: 'Memuat UniDevUI...',
    tabRuntimes: 'Portable Runtimes',
    tabWindowsApps: 'Windows Apps',
    outputLabel: 'Output',
    outputChangeTooltip: 'Klik untuk mengganti folder output ekstraksi arsip',
    btnPackages: 'Packages (.conf)',
    btnPackagesTooltip: 'Buka packages.conf di Notepad (tersimpan di sebelah UniDevUI.exe)',
    btnEssentialsConf: 'Essentials (.conf)',
    btnEssentialsConfTooltip: 'Buka essentials.conf di Notepad (tersimpan di sebelah UniDevUI.exe)',
    btnCheckUpdates: 'Check Updates',
    btnCheckUpdatesTooltip: 'Cek & perbarui link versi LTS/stabil terbaru, CLI tools & status Winget',
    btnReload: 'Reload',
    btnReloadTooltip: 'Muat ulang packages.conf, essentials.conf & status disk/Winget',
    btnOpenFolder: 'Buka Folder',
    btnOpenFolderTooltip: 'Buka folder output ekstraksi di Explorer',
    btnRefreshWinget: 'Refresh Winget',
    searchPlaceholder: 'Cari aplikasi atau tool...',
    searchEssPlaceholder: 'Filter aplikasi Windows...',
    navWorkspace: 'Workspace',
    navAll: 'Semua Paket',
    navCore: 'Core Runtimes',
    navStack: 'Dev Stack',
    navTools: 'CLI & SDK Tools',
    navUserPath: 'User PATH',
    activeInPathTitle: 'Aktif di PATH',
    noActiveInPath: 'Belum ada tool UniDevUI di User PATH.',
    secCoreTitle: 'Core Runtimes & Aplikasi Portable',
    secCoreSubtitle: (baseDir) => `Ekstrak ke ${baseDir} · Aktifkan PATH manual per aplikasi`,
    secStackTitle: 'Dev Stack & Compiler (Arsip Portable)',
    secStackSubtitle: '.NET SDK LTS, Rust, WinLibs GCC, CMake, 7-Zip CLI',
    secToolsTitle: 'Installer CLI & Windows SDK (via CMD / Winget)',
    secToolsSubtitle: 'Wails CLI, Tauri CLI, VS Build Tools 2022 & WebView2 Runtime',
    emptyCore: 'Tidak ada paket core yang cocok.',
    emptyStack: 'Tidak ada stack tambahan yang cocok.',
    emptyTools: 'Tidak ada tool CLI/SDK yang cocok.',
    badgeCore: 'Core',
    badgeStack: 'Dev Stack',
    badgeCLI: 'CLI Tool',
    badgeSDK: 'Windows SDK / Runtime',
    badgeNotInstalled: 'Belum Terpasang',
    badgeExtractedOnly: 'Terekstrak (Belum di PATH)',
    badgeActiveInPath: (ver) => `Aktif di PATH · v${ver}`,
    pathConflict: (scope, fullPath) => `Konflik PATH di ${scope}: ${fullPath} lebih diprioritaskan Windows.`,
    noVersions: '(Tidak ada versi)',
    btnDownload: 'Download & Ekstrak',
    btnReDownload: 'Download Ulang',
    btnDownloadTooltip: (baseDir) => `Unduh arsip & ekstrak ke ${baseDir}`,
    btnNoUrlTooltip: 'URL unduhan belum tersedia di packages.conf',
    downloadSizeLabel: (size) => `Ukuran file unduhan: ${size}`,
    toolSizeWebView2Tip: 'Ukuran installer resmi Edge WebView2 x64 (via Winget)',
    toolSizeVSBuild: (bootSize) =>
        bootSize ? `Bootstrapper ${bootSize} (+ VCTools multi-GB)` : 'Bootstrapper + VCTools multi-GB',
    toolSizeVSBuildTip: 'File awal vs_BuildTools.exe ~4.3 MB, lalu mengunduh komponen MSVC & Windows SDK beberapa GB',
    toolSizeGoDynamic: 'Dinamis (go install)',
    toolSizeGoDynamicTip: 'Mengunduh modul sumber Go secara dinamis & compile lokal (tergantung cache Go Anda)',
    toolSizeCargoHeavy: 'Compile berat (cargo install)',
    toolSizeCargoHeavyTip: 'Mengunduh banyak crate Rust & compile lokal (ratusan MB / beberapa menit)',
    btnAddToPath: 'Add to PATH',
    btnSwitchPath: 'Ganti PATH',
    btnActiveInPath: 'Aktif di PATH',
    tipDownloadFirst: 'Download & Ekstrak versi ini terlebih dahulu sebelum memasukkan ke User PATH',
    tipAlreadyActive: 'Versi ini sudah aktif di User PATH',
    tipAddToPath: 'Tambahkan/ganti entry aplikasi ini di User PATH (HKCU)',
    btnRemovePath: 'Hapus PATH',
    tipRemovePath: 'Hapus aplikasi ini dari User PATH',
    btnDelete: 'Hapus',
    tipDelete: 'Hapus folder hasil ekstrak versi ini',
    btnInstallCmd: 'Install via CMD',
    btnUpdateTool: 'Update',
    btnFixTool: 'Fix',
    tipUpdateAvailable: (ver) => `Perbarui ke v${ver}`,
    tipNoUpdate: 'Sudah versi terbaru (tidak ada update)',
    tipCheckingUpdate: 'Sedang mengecek versi terbaru...',
    tipFixTool: 'Perbaiki atau install ulang tool ini jika bermasalah',
    btnAddDirToPath: (dir) => `Tambah ${dir} ke PATH`,
    locationLabel: 'Lokasi',
    requiresLabel: 'Butuh',
    activeOnWindows: 'Aktif di Windows:',
    installedLabel: 'Terpasang:',
    notInPath: 'Belum di PATH',
    latestLabel: 'Terbaru:',
    latestBranchLabel: (branch) => `Terbaru v${branch}.x:`,
    latestMainLabel: 'Terbaru Utama:',
    checkingLabel: 'Mengecek...',
    statusUpToDate: '✓ Terbaru',
    statusUpToDateBranch: (branch) => `✓ Terbaru di v${branch}.x`,
    statusUpdateTo: (ver) => `↑ Update tersedia (v${ver})`,
    statusUpdateBranch: (branch, ver) => `↑ Update v${branch}.x → v${ver}`,
    statusAhead: '★ Lebih baru dari .conf',
    statusDetected: 'Terdeteksi',
    pathInspectorTitle: 'Inspektor User PATH (HKCU\\Environment\\Path)',
    pathInspectorSubtitle: (count) => `${count} entri · Dievaluasi dari atas ke bawah`,
    pathInspectorEmpty: 'User PATH (HKCU\\Environment) masih kosong.',
    managedBadge: 'Dikelola UniDevUI',
    pathBtnAddFolder: '+ Tambah Folder',
    pathBtnClean: (n) => `Bersihkan Rusak/Duplikat (${n})`,
    pathBtnRefresh: 'Refresh',
    pathBtnMoveUpTip: 'Naikkan urutan (prioritas lebih tinggi)',
    pathBtnMoveDownTip: 'Turunkan urutan (prioritas lebih rendah)',
    pathBtnOpen: 'Buka',
    pathBtnOpenTip: 'Buka folder di Explorer',
    pathBtnRemove: 'Hapus',
    pathBtnRemoveTip: 'Hapus entri ini dari User PATH',
    pathBadgeMissing: 'Hilang',
    pathBadgeDuplicate: 'Duplikat',
    terminalTitle: 'OUTPUT TERMINAL',
    btnCancel: 'Batal',
    btnClear: 'Bersihkan',
    btnMinimize: 'Kecilkan',
    btnExpand: 'Perbesar',
    progressDownloading: 'Mengunduh...',
    progressInstalling: 'Menjalankan installer...',
    progressProcessing: 'Memproses...',
    modalTitle: 'Pengaturan Output Path & Konfigurasi',
    modalDesc: 'Tentukan folder tujuan tempat arsip diekstrak (file .conf tetap di sebelah UniDevUI.exe):',
    modalPlaceholder: 'Misal: D:\\DevTools',
    modalBrowse: 'Pilih...',
    modalActiveConfig: 'Config (di sebelah UniDevUI.exe):',
    modalCopyNote: 'Mengubah Output Path hanya memindahkan lokasi ekstraksi arsip. File packages.conf dan essentials.conf selalu disimpan di folder tempat UniDevUI.exe berada.',
    modalSyncDefaults: 'Sinkron App Bawaan',
    modalSyncTooltip: 'Tambahkan aplikasi bawaan yang kurang ke packages.conf & essentials.conf tanpa menimpa editan Anda',
    modalReset: 'Reset C:\\AppPath',
    modalCancel: 'Batal',
    modalSave: 'Terapkan Path',
    checkedAt: (ts) => `Dicek: ${ts}`,

    essCatAll: 'Semua Aplikasi',
    essCatBrowsers: 'Browser',
    essCatUtilities: 'Arsip & Utilitas',
    essCatEditors: 'Editor & Shell',
    essCatOffice: 'Media & Office',
    essCatChat: 'Chat, Mail & Cloud',
    essCatDownloaders: 'Download Manager',
    essCatSearch: 'Cari di Winget',
    essBatchTitle: 'Antrean Install Massal',
    essSelectedCount: (n) => `${n} aplikasi dipilih`,
    essSelectMissing: 'Pilih Belum Terpasang',
    essClearSel: 'Kosongkan',
    essInstallSelected: (n) => `Install Terpilih (${n})`,
    essWingetReady: (ver) => `Winget Siap (${ver || 'terdeteksi'})`,
    essWingetMissing: 'Winget tidak ditemukan di Windows ini',
    essInstallWinget: 'Install Winget',
    essUacAdmin: 'UAC: Administrator',
    essUacStandard: 'UAC: Standard User (sebagian installer mungkin minta UAC)',
    essRestartAdmin: 'Restart sebagai Admin',
    essConfigStoredHint: (path) => `Config: ${path}`,
    essInstallSilent: 'Install (Silent)',
    essReinstallSilent: 'Install Ulang',
    essBadgeInstalled: (ver) => `Terpasang · v${ver}`,
    essBadgeInstalledNoVer: 'Terpasang',
    essStatusNewerThanWinget: '★ Lebih baru dari Winget',
    essWingetLatestLabel: 'Winget:',
    essBadgeMsStore: 'MS Store',
    essBadgeUAC: 'UAC / Admin',
    essSecSubtitle: 'Instalasi silent aplikasi Windows via Winget · Centang untuk install banyak sekaligus',
    essEmpty: 'Tidak ada aplikasi Windows Apps yang cocok.',
    essSearchInWinget: (q) => `Cari "${q}" di Katalog Winget →`,
    essSearchTitle: 'Pencarian Katalog Winget',
    essSearchSubtitle: 'Cari aplikasi Windows lain di luar daftar 27 bawaan lalu install secara silent',
    essSearchPlaceholder: 'Ketik nama aplikasi atau Winget ID (misal: obsidian, everything, 7zip)...',
    essSearchBtn: 'Cari Winget',
    essSearchEmpty: 'Masukkan kata kunci di atas untuk mencari di katalog resmi Winget & MS Store.',
    essSearching: 'Mencari di katalog Winget...',
    essPinToList: '+ Tambah ke List',
    essAlreadyPinned: 'Di Daftar',
    essVersionLabel: 'Versi:',
    essLaragonNoPopupNote: '6.0.0 (tanpa popup lisensi)',
    essDetailsBtn: 'Detail',
    essDetailsLoading: 'Mengambil metadata Winget & ikon resmi...',
    essEnrichingSearch: 'Memuat penerbit, deskripsi & ikon resmi...',
    essPublisherLabel: 'Penerbit',
    essAuthorLabel: 'Pembuat',
    essHomepageLabel: 'Situs Resmi',
    essLicenseLabel: 'Lisensi',
    essInstallerSizeLabel: 'Ukuran File',
    essInstallerTypeLabel: 'Installer',
    essInstallerUrlLabel: 'URL Installer',
    essSha256Label: 'SHA256',
    essReleaseDateLabel: 'Tanggal Rilis',
    essTagsLabel: 'Tag',
    essDescriptionLabel: 'Deskripsi',
    essOpenHomepage: 'Buka Situs Resmi',
    essModalClose: 'Tutup',
    essDetailsNoData: 'Metadata Winget tambahan belum tersedia untuk paket ini.',
    btnIgnoredConf: 'Ignored (.conf)',
    btnIgnoredConfTooltip: 'Buka ignored_updates.conf di Notepad (tersimpan di sebelah UniDevUI.exe)',
    essCatInstalled: 'Terpasang & Update',
    essInstalledTitle: 'Aplikasi Terpasang & Update',
    essInstalledSubtitle: 'Memindai semua aplikasi terpasang via Winget · Update selektif atau abaikan aplikasi yang dipertahankan',
    essFilterUpdates: (n) => `Perlu Update (${n})`,
    essFilterAllInstalled: (n) => `Semua Terpasang (${n})`,
    essFilterIgnored: (n) => `Diabaikan (${n})`,
    essUpdateAll: (n) => `Update Semua (${n})`,
    essUpdatesEmpty: 'Semua aplikasi non-ignored sudah menggunakan versi terbaru!',
    essInstalledEmpty: 'Tidak ada aplikasi terpasang yang cocok. Klik Refresh Winget untuk memindai ulang.',
    essIgnoredEmpty: 'Belum ada aplikasi yang diabaikan. Klik Abaikan pada aplikasi (mis. Laragon v6) agar tidak ikut ter-update.',
    essBtnIgnore: 'Abaikan',
    essBtnIgnoreTip: 'Sembunyikan dari daftar Update & Terpasang, serta kunci pin Winget Windows (jika didukung)',
    essBtnRestore: 'Pulihkan',
    essBtnRestoreTip: 'Kembalikan ke daftar Terpasang & Update dan hapus pin Winget Windows',
    essBadgeIgnored: 'Diabaikan',
    essBadgeUpdateIgnored: 'Update diabaikan',
    essBadgeWingetPinned: 'Winget Pinned',
    essBadgeLocalEntry: 'Entri Lokal',
    essLocalEntryNoUpdateTip: 'Entri lokal ARP/MSIX tidak memiliki ID repositori Winget; diabaikan lokal di UniDevUI',

    btnAbout: 'Tentang',
    btnAboutTooltip: 'Tentang UniDevUI, versi, pengembang & lisensi',
    aboutTitle: 'Tentang UniDevUI',
    aboutSubtitle: 'Manajer Portable Runtimes & Windows Apps',
    aboutDesc:
        'Utilitas desktop Windows portabel untuk menyiapkan runtime pemrograman secara rapi (manajemen User PATH terisolasi) sekaligus mengotomatiskan instalasi silent, pembaruan, dan penguncian versi aplikasi Windows melalui Winget.',
    aboutVersionLabel: 'Versi',
    aboutAuthorLabel: 'Pengembang',
    aboutEmailLabel: 'Email',
    aboutWebsiteLabel: 'Situs Web',
    aboutTelegramLabel: 'Telegram',
    aboutRepoLabel: 'Repositori GitHub',
    aboutLicenseLabel: 'Lisensi',
    aboutTechLabel: 'Dibangun Dengan',
    aboutAppDirLabel: 'Folder Aplikasi',
    aboutOutputDirLabel: 'Folder Output',
    aboutOpenRepo: 'Buka GitHub',
    aboutOpenTelegram: 'Hubungi Telegram',
    aboutUpdateAvailableBadge: (ver) => `v${ver}`,
    aboutUpdateAvailableTip: (ver) => `UniDevUI v${ver} tersedia! Klik untuk melihat & update.`,
    aboutUpdateTitle: 'Pembaruan Aplikasi',
    aboutUpdateChecking: 'Memeriksa rilis terbaru di GitHub...',
    aboutUpdateUpToDate: (ver) => `UniDevUI v${ver} sudah versi terbaru`,
    aboutUpdateAvailableMsg: (cur, latest) => `Versi baru tersedia: v${cur} → v${latest}`,
    aboutUpdateCheckBtn: 'Cek Update Aplikasi',
    aboutUpdateInstallBtn: 'Update & Restart',
    aboutUpdateOpenReleaseBtn: 'Halaman Rilis',
    aboutUpdateNoAssetNote: 'Aset installer tidak ditemukan pada rilis; silakan unduh lewat Halaman Rilis.',
    aboutUpdateNotesLabel: 'Catatan Rilis',
    aboutUpdateLastChecked: (ts) => `Terakhir dicek: ${ts}`,
};

export function getT(lang: string | undefined): Translations {
    return lang === 'id' ? id : en;
}

const packageDescriptionsID: Record<string, string> = {
    node: 'Runtime JavaScript (otomatis filter jalur LTS resmi)',
    php: 'PHP Windows Non-Thread-Safe (CLI & FastCGI)',
    composer: 'Dependency manager PHP (otomatis buat wrapper composer.bat)',
    go: 'Compiler & toolchain Go (rilis stabil aktif)',
    git: 'Arsip portable Git for Windows (MinGit)',
    pocketbase: 'Backend Go open source dalam 1 file',
    sqlite: 'Shell CLI SQLite (sqlite3.exe), sqldiff & analyzer portable',
    python: 'Build CPython portable oleh Astral (lengkap dengan pip & venv)',
    dotnet: '.NET SDK jalur LTS resmi (arsip zip portable)',
    rust: 'Arsip standalone rustc + cargo (ganti lts_provider = rust-gnu jika tanpa VS Build Tools)',
    gcc: 'Toolchain portable GCC, G++, LLVM & MinGW-w64 UCRT',
    cmake: 'Generator sistem build C/C++ lintas platform',
    '7zip': 'CLI archiver 7zr.exe mandiri (disertai wrapper 7z.bat)',
};

const packageDescriptionsEN: Record<string, string> = {
    node: 'JavaScript runtime (official LTS release channel)',
    php: 'PHP Windows Non-Thread-Safe (CLI & FastCGI)',
    composer: 'PHP dependency manager (auto-creates composer.bat wrapper)',
    go: 'Go compiler & toolchain (active stable releases)',
    git: 'Git for Windows portable archive (MinGit)',
    pocketbase: 'Open source Go backend in 1 file',
    sqlite: 'Portable SQLite CLI shell (sqlite3.exe), sqldiff & analyzer',
    python: 'Portable CPython build by Astral (includes pip & venv)',
    dotnet: '.NET SDK official LTS releases (portable zip)',
    rust: 'Standalone rustc + cargo archive (switch to rust-gnu if no VS Build Tools)',
    gcc: 'Portable GCC, G++, LLVM & MinGW-w64 UCRT toolchain',
    cmake: 'Cross-platform C/C++ build system generator',
    '7zip': 'Standalone 7zr.exe CLI archiver (includes 7z.bat wrapper)',
};

const toolDescriptionsID: Record<string, string> = {
    wails2: 'Build aplikasi desktop Go + WebView2 (wails.exe)',
    wails3: 'CLI Wails generasi terbaru (wails3.exe)',
    tauri: 'Build aplikasi desktop Rust + WebView2 (cargo tauri)',
    vsbuildtools: 'Compiler MSVC (cl.exe), linker & Windows SDK untuk Rust/CGo',
    webview2: 'Runtime WebView2 Evergreen wajib untuk aplikasi Wails & Tauri',
};

const toolRequiresID: Record<string, string> = {
    wails2: 'Membutuhkan Go',
    wails3: 'Membutuhkan Go',
    tauri: 'Membutuhkan Rust + VS Build Tools',
};

const essentialDescriptionsID: Record<string, string> = {
    firefox: 'Browser web open-source berfokus privasi',
    chrome: 'Browser web cepat dari Google',
    brave: 'Browser Chromium dengan pemblokir iklan & pelacak bawaan',
    nanazip: 'Fork 7-Zip modern dengan menu konteks native Windows 11',
    winrar: 'Utilitas arsip klasik RAR & ZIP',
    revo: 'Uninstaller mendalam untuk membersihkan sisa file & registry Windows',
    lightshot: 'Alat screenshot cepat & anotasi ringan',
    stirlingpdf: 'Suite PDF lokal untuk gabung, pecah, konversi & edit PDF',
    powershell7: 'PowerShell 7 modern lintas platform (pwsh)',
    windowsterminal: 'Terminal modern bertab untuk CMD, PowerShell & WSL',
    notepadpp: 'Editor teks & kode sumber yang ringan dan cepat',
    emeditor: 'Editor teks berkecepatan tinggi untuk file CSV/log berukuran besar',
    sublimetext: 'Editor kode ultra cepat',
    vscode: 'IDE & code editor extensible dari Microsoft',
    antigravity: 'IDE Agentic AI dari Google',
    laragon: 'Environment dev universal portable (v6.0.0 tanpa popup lisensi)',
    vlc: 'Pemutar video & audio universal open-source',
    vcredist: 'Library runtime C++ wajib untuk PHP, Apache & aplikasi desktop',
    libreoffice: 'Suite Office gratis & open-source (Writer, Calc, Impress)',
    whatsapp: 'Aplikasi WhatsApp Desktop resmi (via Microsoft Store)',
    telegram: 'Klien pesan desktop cepat',
    discord: 'Klien chat suara, video & komunitas',
    thunderbird: 'Klien email, kalender & RSS open-source',
    googledrive: 'Klien sinkronisasi Google Drive desktop',
    idm: 'Download manager berkecepatan tinggi dengan integrasi browser',
    xdm: 'Akselerator unduhan open-source & pengambil video',
    fdm: 'Download & torrent manager modern gratis',
};

export function localizePkgDesc(pkgId: string, fallback: string, lang: Lang): string {
    if (lang === 'en' && packageDescriptionsEN[pkgId]) {
        return packageDescriptionsEN[pkgId];
    }
    if (lang === 'id' && packageDescriptionsID[pkgId]) {
        return packageDescriptionsID[pkgId];
    }
    return fallback;
}

export function localizeToolDesc(toolId: string, fallback: string, lang: Lang): string {
    if (lang === 'id' && toolDescriptionsID[toolId]) {
        return toolDescriptionsID[toolId];
    }
    return fallback;
}

export function localizeToolRequires(toolId: string, fallback: string | undefined, lang: Lang): string {
    if (!fallback) return '';
    if (lang === 'id' && toolRequiresID[toolId]) {
        return `${getT(lang).requiresLabel}: ${fallback}`;
    }
    return `${getT(lang).requiresLabel}: ${fallback}`;
}

export function localizeEssentialDesc(idKey: string, fallback: string, lang: Lang): string {
    if (lang === 'id' && essentialDescriptionsID[idKey]) {
        return essentialDescriptionsID[idKey];
    }
    return fallback;
}
