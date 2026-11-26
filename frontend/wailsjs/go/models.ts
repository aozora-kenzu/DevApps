export namespace essentials {
	
	export class EnvStatus {
	    wingetAvailable: boolean;
	    wingetVersion: string;
	    wingetPath: string;
	    isAdmin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EnvStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.wingetAvailable = source["wingetAvailable"];
	        this.wingetVersion = source["wingetVersion"];
	        this.wingetPath = source["wingetPath"];
	        this.isAdmin = source["isAdmin"];
	    }
	}
	export class PackageDetails {
	    id: string;
	    name: string;
	    version: string;
	    source: string;
	    publisher: string;
	    publisherUrl: string;
	    author: string;
	    moniker: string;
	    description: string;
	    homepage: string;
	    license: string;
	    licenseUrl: string;
	    releaseDate: string;
	    releaseNotesUrl: string;
	    installerType: string;
	    installerUrl: string;
	    installerSize?: number;
	    installerSha256: string;
	    tags?: string[];
	    iconDataUrl?: string;
	    loaded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackageDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.publisher = source["publisher"];
	        this.publisherUrl = source["publisherUrl"];
	        this.author = source["author"];
	        this.moniker = source["moniker"];
	        this.description = source["description"];
	        this.homepage = source["homepage"];
	        this.license = source["license"];
	        this.licenseUrl = source["licenseUrl"];
	        this.releaseDate = source["releaseDate"];
	        this.releaseNotesUrl = source["releaseNotesUrl"];
	        this.installerType = source["installerType"];
	        this.installerUrl = source["installerUrl"];
	        this.installerSize = source["installerSize"];
	        this.installerSha256 = source["installerSha256"];
	        this.tags = source["tags"];
	        this.iconDataUrl = source["iconDataUrl"];
	        this.loaded = source["loaded"];
	    }
	}
	export class WingetTableRow {
	    name: string;
	    id: string;
	    version: string;
	    available?: string;
	    match?: string;
	    source?: string;
	    installed?: boolean;
	    installedVer?: string;
	    hasUpdate?: boolean;
	    matchedBy?: string;
	
	    static createFrom(source: any = {}) {
	        return new WingetTableRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.id = source["id"];
	        this.version = source["version"];
	        this.available = source["available"];
	        this.match = source["match"];
	        this.source = source["source"];
	        this.installed = source["installed"];
	        this.installedVer = source["installedVer"];
	        this.hasUpdate = source["hasUpdate"];
	        this.matchedBy = source["matchedBy"];
	    }
	}

}

export namespace main {
	
	export class BatchEssentialItem {
	    id: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new BatchEssentialItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	    }
	}
	export class TerminalLogEvent {
	    timestamp: string;
	    level: string;
	    message: string;
	    messageId?: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalLogEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.messageId = source["messageId"];
	    }
	}
	export class InstalledAppView {
	    id: string;
	    name: string;
	    installedVer: string;
	    availableVer: string;
	    source: string;
	    hasUpdate: boolean;
	    ignored: boolean;
	    wingetPinned: boolean;
	    ignoreReason?: string;
	    managedByWinget: boolean;
	    pinnedCurated: boolean;
	    curatedAppId?: string;
	    downloadSize: number;
	
	    static createFrom(source: any = {}) {
	        return new InstalledAppView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.installedVer = source["installedVer"];
	        this.availableVer = source["availableVer"];
	        this.source = source["source"];
	        this.hasUpdate = source["hasUpdate"];
	        this.ignored = source["ignored"];
	        this.wingetPinned = source["wingetPinned"];
	        this.ignoreReason = source["ignoreReason"];
	        this.managedByWinget = source["managedByWinget"];
	        this.pinnedCurated = source["pinnedCurated"];
	        this.curatedAppId = source["curatedAppId"];
	        this.downloadSize = source["downloadSize"];
	    }
	}
	export class EssentialView {
	    id: string;
	    name: string;
	    category: string;
	    wingetId: string;
	    source: string;
	    requiresUac: boolean;
	    versions?: string[];
	    description: string;
	    installed: boolean;
	    installedVer: string;
	    availableVer: string;
	    hasUpdate: boolean;
	    updateIgnored: boolean;
	    versionStatus: string;
	    matchedBy?: string;
	    downloadSize: number;
	
	    static createFrom(source: any = {}) {
	        return new EssentialView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.wingetId = source["wingetId"];
	        this.source = source["source"];
	        this.requiresUac = source["requiresUac"];
	        this.versions = source["versions"];
	        this.description = source["description"];
	        this.installed = source["installed"];
	        this.installedVer = source["installedVer"];
	        this.availableVer = source["availableVer"];
	        this.hasUpdate = source["hasUpdate"];
	        this.updateIgnored = source["updateIgnored"];
	        this.versionStatus = source["versionStatus"];
	        this.matchedBy = source["matchedBy"];
	        this.downloadSize = source["downloadSize"];
	    }
	}
	export class VersionView {
	    version: string;
	    url: string;
	    downloadSize: number;
	    installed: boolean;
	    active: boolean;
	    binDir?: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.url = source["url"];
	        this.downloadSize = source["downloadSize"];
	        this.installed = source["installed"];
	        this.active = source["active"];
	        this.binDir = source["binDir"];
	    }
	}
	export class PackageView {
	    id: string;
	    name: string;
	    category: string;
	    binSubdir: string;
	    binary: string;
	    ltsProvider: string;
	    description: string;
	    activeVer: string;
	    activeDir: string;
	    latestVer: string;
	    branchPrefix: string;
	    branchLatestVer: string;
	    branchStatus: string;
	    winActiveVer: string;
	    winActivePath: string;
	    winActiveScope: string;
	    versionStatus: string;
	    conflict?: pathmgr.ConflictInfo;
	    versions: VersionView[];
	
	    static createFrom(source: any = {}) {
	        return new PackageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.binSubdir = source["binSubdir"];
	        this.binary = source["binary"];
	        this.ltsProvider = source["ltsProvider"];
	        this.description = source["description"];
	        this.activeVer = source["activeVer"];
	        this.activeDir = source["activeDir"];
	        this.latestVer = source["latestVer"];
	        this.branchPrefix = source["branchPrefix"];
	        this.branchLatestVer = source["branchLatestVer"];
	        this.branchStatus = source["branchStatus"];
	        this.winActiveVer = source["winActiveVer"];
	        this.winActivePath = source["winActivePath"];
	        this.winActiveScope = source["winActiveScope"];
	        this.versionStatus = source["versionStatus"];
	        this.conflict = this.convertValues(source["conflict"], pathmgr.ConflictInfo);
	        this.versions = this.convertValues(source["versions"], VersionView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DashboardState {
	    appDir: string;
	    baseDir: string;
	    confPath: string;
	    essentialsConfPath: string;
	    ignoredConfPath: string;
	    packages: PackageView[];
	    tools: tools.ToolStatus[];
	    essentials: EssentialView[];
	    installedApps: InstalledAppView[];
	    wingetEnv: essentials.EnvStatus;
	    userPathList: string[];
	    userPathEntries: pathmgr.UserPathEntry[];
	    logs: TerminalLogEvent[];
	    busy: boolean;
	    lastLtsCheck: string;
	    language: string;
	    appVersion: string;
	    appUpdate: selfupdate.UpdateInfo;
	
	    static createFrom(source: any = {}) {
	        return new DashboardState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appDir = source["appDir"];
	        this.baseDir = source["baseDir"];
	        this.confPath = source["confPath"];
	        this.essentialsConfPath = source["essentialsConfPath"];
	        this.ignoredConfPath = source["ignoredConfPath"];
	        this.packages = this.convertValues(source["packages"], PackageView);
	        this.tools = this.convertValues(source["tools"], tools.ToolStatus);
	        this.essentials = this.convertValues(source["essentials"], EssentialView);
	        this.installedApps = this.convertValues(source["installedApps"], InstalledAppView);
	        this.wingetEnv = this.convertValues(source["wingetEnv"], essentials.EnvStatus);
	        this.userPathList = source["userPathList"];
	        this.userPathEntries = this.convertValues(source["userPathEntries"], pathmgr.UserPathEntry);
	        this.logs = this.convertValues(source["logs"], TerminalLogEvent);
	        this.busy = source["busy"];
	        this.lastLtsCheck = source["lastLtsCheck"];
	        this.language = source["language"];
	        this.appVersion = source["appVersion"];
	        this.appUpdate = this.convertValues(source["appUpdate"], selfupdate.UpdateInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	export class WingetUpdateTarget {
	    name: string;
	    wingetId: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new WingetUpdateTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.wingetId = source["wingetId"];
	        this.source = source["source"];
	    }
	}

}

export namespace pathmgr {
	
	export class ConflictInfo {
	    scope: string;
	    fullPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.fullPath = source["fullPath"];
	    }
	}
	export class UserPathEntry {
	    index: number;
	    path: string;
	    expanded: string;
	    exists: boolean;
	    managed: boolean;
	    duplicate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UserPathEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.path = source["path"];
	        this.expanded = source["expanded"];
	        this.exists = source["exists"];
	        this.managed = source["managed"];
	        this.duplicate = source["duplicate"];
	    }
	}

}

export namespace selfupdate {
	
	export class UpdateInfo {
	    currentVersion: string;
	    latestVersion: string;
	    tagName: string;
	    hasUpdate: boolean;
	    releaseUrl: string;
	    releaseNotes: string;
	    publishedAt: string;
	    assetName: string;
	    downloadUrl: string;
	    downloadSize: number;
	    sha256: string;
	    checkedAt: string;
	    checkError?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.tagName = source["tagName"];
	        this.hasUpdate = source["hasUpdate"];
	        this.releaseUrl = source["releaseUrl"];
	        this.releaseNotes = source["releaseNotes"];
	        this.publishedAt = source["publishedAt"];
	        this.assetName = source["assetName"];
	        this.downloadUrl = source["downloadUrl"];
	        this.downloadSize = source["downloadSize"];
	        this.sha256 = source["sha256"];
	        this.checkedAt = source["checkedAt"];
	        this.checkError = source["checkError"];
	    }
	}

}

export namespace tools {
	
	export class ToolStatus {
	    id: string;
	    name: string;
	    category: string;
	    installed: boolean;
	    version: string;
	    rawVersion: string;
	    latestVersion: string;
	    versionStatus: string;
	    location: string;
	    binDir?: string;
	    inPath: boolean;
	    description: string;
	    commandHint: string;
	    requires?: string;
	    downloadSize: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.rawVersion = source["rawVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.versionStatus = source["versionStatus"];
	        this.location = source["location"];
	        this.binDir = source["binDir"];
	        this.inPath = source["inPath"];
	        this.description = source["description"];
	        this.commandHint = source["commandHint"];
	        this.requires = source["requires"];
	        this.downloadSize = source["downloadSize"];
	    }
	}

}

