export interface CuratedMeta {
    publisher: string;
    domain: string;
    license: string;
}

const curatedMetaByKey: Record<string, CuratedMeta> = {
    firefox: { publisher: 'Mozilla', domain: 'mozilla.org', license: 'MPL-2.0' },
    chrome: { publisher: 'Google LLC', domain: 'google.com/chrome', license: 'Freeware' },
    brave: { publisher: 'Brave Software Inc.', domain: 'brave.com', license: 'MPL-2.0' },
    nanazip: { publisher: 'M2-Team', domain: 'github.com/M2Team/NanaZip', license: 'MIT' },
    winrar: { publisher: 'win.rar GmbH', domain: 'rarlab.com', license: 'Shareware' },
    revo: { publisher: 'VS Revo Group', domain: 'revouninstaller.com', license: 'Freeware' },
    lightshot: { publisher: 'Skillbrains', domain: 'app.prntscr.com', license: 'Freeware' },
    stirlingpdf: { publisher: 'Stirling Software', domain: 'stirlingpdf.com', license: 'GPL-3.0' },
    powershell7: { publisher: 'Microsoft Corporation', domain: 'github.com/PowerShell', license: 'MIT' },
    windowsterminal: { publisher: 'Microsoft Corporation', domain: 'aka.ms/terminal', license: 'MIT' },
    notepadpp: { publisher: 'Notepad++ Team', domain: 'notepad-plus-plus.org', license: 'GPL-3.0' },
    emeditor: { publisher: 'Emurasoft, Inc.', domain: 'emeditor.com', license: 'Proprietary' },
    sublimetext: { publisher: 'Sublime HQ Pty Ltd', domain: 'sublimetext.com', license: 'Proprietary' },
    vscode: { publisher: 'Microsoft Corporation', domain: 'code.visualstudio.com', license: 'MIT / Proprietary' },
    antigravity: { publisher: 'Google LLC', domain: 'antigravity.google', license: 'Proprietary' },
    laragon: { publisher: 'Le Ngoc Khoa', domain: 'laragon.org', license: 'Proprietary' },
    vlc: { publisher: 'VideoLAN', domain: 'videolan.org', license: 'GPL-2.0+' },
    vcredist: { publisher: 'Microsoft Corporation', domain: 'learn.microsoft.com', license: 'Proprietary' },
    libreoffice: { publisher: 'The Document Foundation', domain: 'libreoffice.org', license: 'MPL-2.0' },
    whatsapp: { publisher: 'WhatsApp Inc. (Meta)', domain: 'whatsapp.com', license: 'Freeware (MS Store)' },
    telegram: { publisher: 'Telegram FZ-LLC', domain: 'desktop.telegram.org', license: 'GPL-3.0' },
    discord: { publisher: 'Discord Inc.', domain: 'discord.com', license: 'Proprietary' },
    thunderbird: { publisher: 'Mozilla Foundation', domain: 'thunderbird.net', license: 'MPL-2.0' },
    googledrive: { publisher: 'Google LLC', domain: 'google.com/drive', license: 'Proprietary' },
    idm: { publisher: 'Tonec Inc.', domain: 'internetdownloadmanager.com', license: 'Shareware' },
    xdm: { publisher: 'Subhra Das Gupta', domain: 'github.com/subhra74/xdm', license: 'GPL-2.0' },
    fdm: { publisher: 'SoftDeluxe', domain: 'freedownloadmanager.org', license: 'Freeware' },
};

export function getCuratedMeta(id: string): CuratedMeta | undefined {
    return curatedMetaByKey[id.toLowerCase()];
}

// Built-in crisp 28x28 SVG vectors inside a uniform 36x36 rounded tile.
const svgByKey: Record<string, string> = {
    // --- Tab 1: Core Runtimes & Dev Stack ---
    node: `<svg viewBox="0 0 32 32" fill="none"><polygon points="16,2 28,9 28,23 16,30 4,23 4,9" fill="#539E43"/><path d="M16 6l8.5 5v10L16 26l-8.5-5V11L16 6z" fill="#3E863D"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="9" font-weight="800" font-family="sans-serif">JS</text></svg>`,
    php: `<svg viewBox="0 0 32 32" fill="none"><ellipse cx="16" cy="16" rx="14" ry="9" fill="#777BB3"/><ellipse cx="16" cy="16" rx="12.5" ry="7.6" fill="#4F5B93"/><text x="16" y="19" text-anchor="middle" fill="#fff" font-size="9.5" font-weight="800" font-style="italic" font-family="sans-serif">php</text></svg>`,
    composer: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#885630"/><path d="M10 22V11l6-3 6 3v11l-6 2.5L10 22z" fill="#F2D7B6"/><circle cx="16" cy="15" r="2.2" fill="#885630"/><path d="M18.5 11l4.5-3" stroke="#FFD166" stroke-width="2.2" stroke-linecap="round"/></svg>`,
    go: `<svg viewBox="0 0 32 32" fill="none"><rect x="2" y="7" width="28" height="18" rx="9" fill="#00ADD8"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="11" font-weight="800" font-style="italic" font-family="sans-serif">Go</text></svg>`,
    git: `<svg viewBox="0 0 32 32" fill="none"><rect x="5" y="5" width="22" height="22" rx="4" transform="rotate(45 16 16)" fill="#F05032"/><path d="M13 10l6.5 6.5M16 13v8" stroke="#fff" stroke-width="2" stroke-linecap="round"/><circle cx="13" cy="10" r="2.1" fill="#fff"/><circle cx="19.5" cy="16.5" r="2.1" fill="#fff"/><circle cx="16" cy="21" r="2.1" fill="#fff"/></svg>`,
    pocketbase: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#1E293B" stroke="#38BDF8" stroke-width="1.5"/><path d="M16 8l7 4v8l-7 4-7-4v-8l7-4z" fill="#38BDF8" opacity=".28"/><path d="M16 8v16M9 12l7 4 7-4" stroke="#7DD3FC" stroke-width="2" stroke-linejoin="round"/></svg>`,
    sqlite: `<svg viewBox="0 0 32 32" fill="none"><rect x="5" y="4" width="22" height="24" rx="5" fill="#0F80CC"/><path d="M9 11c3.5-2 10.5-2 14 0M9 16c3.5-2 10.5-2 14 0M9 21c3.5-2 10.5-2 14 0" stroke="#BCE3FF" stroke-width="2" stroke-linecap="round"/><path d="M19 7c3 4 4 9 3 17" stroke="#fff" stroke-width="1.8" stroke-linecap="round"/></svg>`,
    python: `<svg viewBox="0 0 32 32" fill="none"><path d="M15.8 4C11 4 11.4 6.1 11.4 6.1l.1 3h4.5v1H9.6S6.5 9.7 6.5 14.5s2.7 4.7 2.7 4.7h1.6v-2.4s-.1-2.7 2.7-2.7h4.5s2.5 0 2.5-2.4V6.6S20.9 4 15.8 4z" fill="#3776AB"/><circle cx="13.5" cy="6.8" r="1" fill="#fff"/><path d="M16.2 28c4.8 0 4.4-2.1 4.4-2.1l-.1-3H16v-1h6.4s3.1.4 3.1-4.4-2.7-4.7-2.7-4.7h-1.6v2.4s.1 2.7-2.7 2.7H14s-2.5 0-2.5 2.4v5.1s-.4 2.6 4.7 2.6z" fill="#FFD43B"/><circle cx="18.5" cy="25.2" r="1" fill="#1F2937"/></svg>`,
    dotnet: `<svg viewBox="0 0 32 32" fill="none"><rect x="3" y="5" width="26" height="22" rx="5" fill="#512BD4"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="9" font-weight="800" font-family="sans-serif">.NET</text></svg>`,
    rust: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="11" fill="#B7410E"/><circle cx="16" cy="16" r="7.5" fill="#1C1E26" stroke="#F4A261" stroke-width="2"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="11" font-weight="800" font-family="serif">R</text></svg>`,
    gcc: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#00599C"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="9.5" font-weight="800" font-family="monospace">C++</text></svg>`,
    cmake: `<svg viewBox="0 0 32 32" fill="none"><polygon points="16,4 4,26 16,18" fill="#064F8C"/><polygon points="16,4 28,26 18,20" fill="#D22128"/><polygon points="4,26 28,26 17,20" fill="#01A300"/></svg>`,
    '7zip': `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="5" width="24" height="22" rx="4" fill="#1E293B" stroke="#94A3B8" stroke-width="1.4"/><rect x="4" y="5" width="12" height="22" rx="3" fill="#E2E8F0"/><text x="10" y="20" text-anchor="middle" fill="#0F172A" font-size="12" font-weight="900" font-family="sans-serif">7</text><text x="22" y="20" text-anchor="middle" fill="#F8FAFC" font-size="10" font-weight="800" font-family="sans-serif">z</text></svg>`,

    // --- Tab 1: CLI & SDK Tools ---
    wails2: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#B91C1C"/><path d="M7.5 11l3.5 11 5-8 5 8 3.5-11" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    wails3: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#DC2626"/><path d="M7.5 11l3.5 11 5-8 5 8 3.5-11" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/><circle cx="25" cy="7" r="4" fill="#FACC15"/></svg>`,
    tauri: `<svg viewBox="0 0 32 32" fill="none"><circle cx="12" cy="11" r="4.5" fill="#FFC131"/><circle cx="20" cy="21" r="4.5" fill="#24C8DB"/><path d="M18 7a9 9 0 017 10M14 25a9 9 0 01-7-10" stroke="#94A3B8" stroke-width="2.2" stroke-linecap="round"/></svg>`,
    vsbuildtools: `<svg viewBox="0 0 32 32" fill="none"><path d="M23 4l5 2.5v19L23 28l-10.5-9.5-5.5 4.2-2.5-1.2V10.5L7 9.3l5.5 4.2L23 4z" fill="#854CC7"/><path d="M23 11.2L16.5 16l6.5 4.8v-9.6z" fill="#C084FC"/></svg>`,
    webview2: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#0078D4"/><path d="M7 18c2.5-5 10-7.5 17-3.5-2-4.5-6.5-6.5-11-5.5C9.5 10 7 14 7 18z" fill="#38BDF8"/><circle cx="18" cy="19" r="5" fill="#10B981" opacity=".9"/></svg>`,

    // --- Tab 2: Curated 27 Windows Apps ---
    firefox: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="11" fill="#0060DF"/><path d="M25 10c-2-3.5-6-5-9.5-4.5 2.5 1 4 3 3.5 5.5-2-1-4.5-.5-6 1.5C11 9.5 9 7 9.5 4.5 5.5 7 4 12.5 5.5 17.5 7.5 24 14.5 27.5 21 25.5c5.5-1.8 8.5-8 4-15.5z" fill="#FF7139"/><circle cx="16" cy="16" r="4.5" fill="#80C8FF" opacity=".7"/></svg>`,
    chrome: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#FBBC05"/><path d="M16 4a12 12 0 0110.4 6H16l-5.2 9L5.6 10A12 12 0 0116 4z" fill="#EA4335"/><path d="M5.6 10l5.2 9 4 7A12 12 0 015.6 10z" fill="#34A853"/><circle cx="16" cy="16" r="5.5" fill="#fff"/><circle cx="16" cy="16" r="4.1" fill="#4285F4"/></svg>`,
    brave: `<svg viewBox="0 0 32 32" fill="none"><path d="M16 3l10 3.5-1.5 11.5L16 29 7.5 18 6 6.5 16 3z" fill="#FB542B"/><path d="M16 8l5 2.5-1 5.5-4 3-4-3-1-5.5L16 8z" fill="#fff"/><path d="M13 13l3 2 3-2-1 4-2 1.5L14 17l-1-4z" fill="#FB542B"/></svg>`,
    nanazip: `<svg viewBox="0 0 32 32" fill="none"><rect x="5" y="4" width="22" height="24" rx="5" fill="#2563EB"/><rect x="14" y="4" width="4" height="11" fill="#93C5FD"/><rect x="13.5" y="13" width="5" height="4" rx="1" fill="#fff"/><text x="16" y="24.5" text-anchor="middle" fill="#fff" font-size="8.5" font-weight="800" font-family="sans-serif">NZ</text></svg>`,
    winrar: `<svg viewBox="0 0 32 32" fill="none"><rect x="6" y="4" width="20" height="7.5" rx="1.5" fill="#9333EA"/><rect x="6" y="12.2" width="20" height="7.5" rx="1.5" fill="#2563EB"/><rect x="6" y="20.5" width="20" height="7.5" rx="1.5" fill="#16A34A"/><rect x="13.5" y="3" width="5" height="26" rx="1" fill="#FACC15" opacity=".9"/></svg>`,
    powershell7: `<svg viewBox="0 0 32 32" fill="none"><path d="M6 7h22l-4 18H2L6 7z" fill="#1E293B" stroke="#38BDF8" stroke-width="1.6"/><path d="M9.5 12l5 4-6.5 4M15 20h5.5" stroke="#38BDF8" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    windowsterminal: `<svg viewBox="0 0 32 32" fill="none"><rect x="3" y="5" width="26" height="22" rx="4" fill="#27272A" stroke="#52525B" stroke-width="1.5"/><path d="M3 9.5h26" stroke="#52525B" stroke-width="1.5"/><path d="M8.5 14.5l4.5 3.5-4.5 3.5M14.5 21.5h5.5" stroke="#fff" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    notepadpp: `<svg viewBox="0 0 32 32" fill="none"><rect x="6" y="4" width="20" height="24" rx="3" fill="#84CC16"/><rect x="9" y="8" width="14" height="16" rx="1.5" fill="#ECFCCB"/><path d="M18 6l7 7-9 9H13v-3l5-13z" fill="#3F6212"/></svg>`,
    emeditor: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="5" fill="#1D4ED8"/><text x="16" y="20" text-anchor="middle" fill="#fff" font-size="11" font-weight="800" font-family="sans-serif">Em</text></svg>`,
    sublimetext: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="5" fill="#374151"/><path d="M9 12l14-4v5L9 17v-5zm0 8l14-4v5L9 25v-5z" fill="#FF9800"/><path d="M9 17l14-4v5L9 22v-5z" fill="#FFB74D" opacity=".8"/></svg>`,
    vscode: `<svg viewBox="0 0 32 32" fill="none"><path d="M23.5 3.5L28 5.8v20.4l-4.5 2.3-11-9.8-5.5 4.2-2.5-1.3V10.4L7 9.1l5.5 4.2 11-9.8z" fill="#007ACC"/><path d="M23.5 10.2L16 16l7.5 5.8V10.2z" fill="#38BDF8"/></svg>`,
    antigravity: `<svg viewBox="0 0 32 32" fill="none"><rect x="3" y="3" width="26" height="26" rx="7" fill="#1E293B" stroke="#60A5FA" stroke-width="1.4"/><path d="M16 6l8 18h-4.5l-3.5-8.5L12.5 24H8l8-18z" fill="#60A5FA"/><circle cx="16" cy="21" r="2.2" fill="#C084FC"/></svg>`,
    vlc: `<svg viewBox="0 0 32 32" fill="none"><path d="M13 5h6l4.5 17h-15L13 5z" fill="#FF7800"/><path d="M11.6 10h8.8l1 3.6H10.6l1-3.6zm-2 7h12.8l1 3.6H8.6l1-3.6z" fill="#fff"/><rect x="5" y="23" width="22" height="4" rx="1.5" fill="#EA580C"/></svg>`,
    vcredist: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="5" fill="#6D28D9"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="9" font-weight="800" font-family="sans-serif">VC++</text></svg>`,
    libreoffice: `<svg viewBox="0 0 32 32" fill="none"><path d="M6 4h13l7 7v17H6V4z" fill="#18A303"/><path d="M8.5 6.5h9.5l5.5 5.5v13.5h-15V6.5z" fill="#fff"/><path d="M20 4h6v6l-6-6z" fill="#106B02"/></svg>`,
    revo: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#1D4ED8"/><path d="M10 10h6.5a4 4 0 010 8H10V10z" stroke="#fff" stroke-width="2.4"/><path d="M15 18l6.5 6.5" stroke="#F87171" stroke-width="3" stroke-linecap="round"/></svg>`,
    whatsapp: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="15.5" r="11.5" fill="#25D366"/><path d="M8 25l1.6-4.5" stroke="#25D366" stroke-width="4" stroke-linecap="round"/><path d="M12.5 11.5c.6-.6 1.5-.3 1.9.4l.8 1.5c.2.5 0 1-.4 1.3l-.6.6c.7 1.3 1.8 2.4 3.1 3.1l.6-.6c.3-.4.8-.6 1.3-.4l1.5.8c.7.4 1 1.3.4 1.9-.9 1-2.5 1.3-4.2.5-2.6-1.2-4.7-3.3-5.9-5.9-.8-1.7-.5-3.3.5-4.2z" fill="#fff"/></svg>`,
    telegram: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#229ED9"/><path d="M22.5 10.2l-2.7 12.6c-.2.9-.8 1.1-1.6.7l-4.4-3.2-2.1 2c-.2.2-.4.4-.9.4l.3-4.5 8.2-7.4c.4-.3-.1-.5-.6-.2l-10.1 6.4-4.3-1.4c-.9-.3-.9-.9.2-1.3l16.9-6.5c.8-.3 1.5.2 1.1 1.4z" fill="#fff"/></svg>`,
    discord: `<svg viewBox="0 0 32 32" fill="none"><rect x="3" y="4" width="26" height="24" rx="7" fill="#5865F2"/><path d="M21.8 10.5A14 14 0 0018.4 9.5l-.4.8a13 13 0 00-4 0l-.4-.8a14 14 0 00-3.4 1C8.1 13.7 7.5 16.8 7.8 19.8a14 14 0 004.2 2.1l.9-1.4a9 9 0 01-1.4-.7l.3-.3a10 10 0 008.4 0l.3.3c-.4.3-.9.5-1.4.7l.9 1.4a14 14 0 004.2-2.1c.4-3.5-.6-6.6-2.4-9.3z" fill="#fff"/><circle cx="13" cy="16.2" r="1.5" fill="#5865F2"/><circle cx="19" cy="16.2" r="1.5" fill="#5865F2"/></svg>`,
    thunderbird: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#0A84FF"/><path d="M8 14l8 5 8-5v8H8v-8z" fill="#fff"/><path d="M8 12.5l8 5 8-5" stroke="#7DD3FC" stroke-width="2"/></svg>`,
    googledrive: `<svg viewBox="0 0 32 32" fill="none"><polygon points="12,5 20,5 27,18 19,18" fill="#FBBC05"/><polygon points="5,18 12,5 16,12 9,25" fill="#34A853"/><polygon points="9,25 27,25 23,18 13,18" fill="#4285F4"/></svg>`,
    laragon: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#0EA5E9"/><path d="M11 9v14h11" stroke="#fff" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"/><circle cx="20" cy="12" r="2.2" fill="#BAE6FD"/></svg>`,
    idm: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#16A34A"/><path d="M16 8v11m0 0l-5-5m5 5l5-5M9 22.5h14" stroke="#fff" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    xdm: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#0284C7"/><path d="M10 10l12 12M22 10L10 22" stroke="#fff" stroke-width="2.6" stroke-linecap="round"/><path d="M16 17v7m0 0l-3-3m3 3l3-3" stroke="#FACC15" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    fdm: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#2563EB"/><path d="M16 9v11m0 0l-4.5-4.5M16 20l4.5-4.5" stroke="#fff" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`,
    lightshot: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#7E22CE"/><path d="M23 8C16 9 11 14 9 23c4-1 7-2 9-4s3-5 5-11z" fill="#E9D5FF"/><path d="M9 23L19 12" stroke="#7E22CE" stroke-width="1.6" stroke-linecap="round"/></svg>`,
    stirlingpdf: `<svg viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="24" height="24" rx="6" fill="#E11D48"/><text x="16" y="19.5" text-anchor="middle" fill="#fff" font-size="9.5" font-weight="800" font-family="sans-serif">PDF</text></svg>`,

    // --- Bonus recognizable icons for common Winget Search results ---
    obsidian: `<svg viewBox="0 0 32 32" fill="none"><polygon points="15,3 25,11 21,28 11,26 6,16" fill="#7C3AED"/><polygon points="15,3 25,11 17,19 11,12" fill="#A78BFA"/></svg>`,
    docker: `<svg viewBox="0 0 32 32" fill="none"><rect x="3" y="4" width="26" height="24" rx="6" fill="#0EA5E9"/><path d="M7 16h18c0 5-4 8-10 8s-8-3-8-8z" fill="#fff"/><rect x="10" y="11" width="3" height="3" fill="#fff"/><rect x="14" y="11" width="3" height="3" fill="#fff"/><rect x="18" y="11" width="3" height="3" fill="#fff"/><rect x="14" y="7.5" width="3" height="3" fill="#fff"/></svg>`,
    github: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#24292F"/><path d="M16 9a7 7 0 00-2.2 13.6c.4.1.5-.2.5-.4v-1.4c-2 .4-2.4-1-2.4-1-.3-.8-.8-1-.8-1-.7-.5.1-.5.1-.5.7.1 1.1.8 1.1.8.7 1.1 1.7.8 2.1.6.1-.5.3-.8.5-1-1.6-.2-3.3-.8-3.3-3.6 0-.8.3-1.4.8-1.9-.1-.2-.3-.9.1-1.9 0 0 .6-.2 2 .8a6.7 6.7 0 013.6 0c1.4-1 2-.8 2-.8.4 1 .2 1.7.1 1.9.5.5.8 1.1.8 1.9 0 2.8-1.7 3.4-3.3 3.6.3.2.5.7.5 1.4v2c0 .2.1.5.5.4A7 7 0 0016 9z" fill="#fff"/></svg>`,
    spotify: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#1DB954"/><path d="M10 13c4-1 9-.6 12.5 1.2M11 17c3.2-.8 7.5-.4 10.2 1M12 20.5c2.5-.6 5.5-.3 7.5.7" stroke="#0F172A" stroke-width="2" stroke-linecap="round"/></svg>`,
    steam: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#1B2838"/><circle cx="19" cy="12" r="4" stroke="#66C0F4" stroke-width="2"/><circle cx="11" cy="20" r="2.8" fill="#66C0F4"/><path d="M11 20l6.5-6" stroke="#66C0F4" stroke-width="2.5" stroke-linecap="round"/></svg>`,
    edge: `<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" fill="#0078D4"/><path d="M8 18c2.5-5 10-7.5 16-3.5-2-4.5-6.5-6.5-11-5.5C9.5 10 7.5 14 8 18z" fill="#38BDF8"/><circle cx="18" cy="19" r="4.8" fill="#10B981"/></svg>`,
};

const wingetAliasMap: Record<string, string> = {
    'mozilla.firefox': 'firefox',
    'google.chrome': 'chrome',
    'brave.brave': 'brave',
    'm2team.nanazip': 'nanazip',
    'rarlab.winrar': 'winrar',
    'revouninstaller.revouninstaller': 'revo',
    'skillbrains.lightshot': 'lightshot',
    'stirlingtools.stirlingpdf': 'stirlingpdf',
    'microsoft.powershell': 'powershell7',
    'microsoft.windowsterminal': 'windowsterminal',
    'notepad++.notepad++': 'notepadpp',
    'emurasoft.emeditor': 'emeditor',
    'sublimehq.sublimetext.4': 'sublimetext',
    'microsoft.visualstudiocode': 'vscode',
    'microsoft.visualstudiocode.insiders': 'vscode',
    'google.antigravity': 'antigravity',
    'lengockhoa.laragon': 'laragon',
    'videolan.vlc': 'vlc',
    'microsoft.vcredist.2015+.x64': 'vcredist',
    'thedocumentfoundation.libreoffice': 'libreoffice',
    '9nksqgp7f2nh': 'whatsapp',
    'telegram.telegramdesktop': 'telegram',
    'discord.discord': 'discord',
    'mozilla.thunderbird': 'thunderbird',
    'google.googledrive': 'googledrive',
    'tonec.internetdownloadmanager': 'idm',
    'subhra74.xtremedownloadmanager': 'xdm',
    'softdeluxe.freedownloadmanager': 'fdm',
    '7zip.7zip': '7zip',
    'git.git': 'git',
    'openjs.nodejs': 'node',
    'openjs.nodejs.lts': 'node',
    'golang.go': 'go',
    'python.python.3.12': 'python',
    'python.python.3.13': 'python',
    'obsidian.obsidian': 'obsidian',
    'docker.dockerdesktop': 'docker',
    'github.githubdesktop': 'github',
    'github.cli': 'github',
    'spotify.spotify': 'spotify',
    'valve.steam': 'steam',
    'microsoft.edge': 'edge',
};

const paletteColors = ['#2563EB', '#0EA5E9', '#10B981', '#8B5CF6', '#EC4899', '#F59E0B', '#EF4444', '#14B8A6'];

export function lookupBuiltInSvg(idOrWingetId: string, name?: string): string {
    const k = idOrWingetId.trim().toLowerCase();
    if (svgByKey[k]) return svgByKey[k];
    if (wingetAliasMap[k] && svgByKey[wingetAliasMap[k]]) {
        return svgByKey[wingetAliasMap[k]];
    }
    const lowerName = (name || '').toLowerCase();
    if (k.startsWith('python.python') || lowerName.startsWith('python 3')) return svgByKey.python;
    if (k.startsWith('rust') || lowerName.includes('rust ')) return svgByKey.rust;
    if (k.includes('visualstudio') && k.includes('buildtools')) return svgByKey.vsbuildtools;
    if (k.startsWith('microsoft.edge')) return svgByKey.edge;
    if (k.startsWith('jetbrains.') || k.startsWith('github.')) return svgByKey.github;
    return '';
}

function monogramColor(seed: string): string {
    let hash = 0;
    for (let i = 0; i < seed.length; i++) {
        hash = (hash * 31 + seed.charCodeAt(i)) | 0;
    }
    return paletteColors[Math.abs(hash) % paletteColors.length];
}

function makeMonogram(name: string): string {
    const words = name
        .replace(/[^a-zA-Z0-9\s]+/g, ' ')
        .trim()
        .split(/\s+/)
        .filter(Boolean);
    if (words.length >= 2) {
        return `${words[0][0]}${words[1][0]}`.toUpperCase();
    }
    if (words.length === 1) {
        return words[0].slice(0, 2).toUpperCase();
    }
    return 'AP';
}

function escapeAttr(val: string): string {
    return val.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

export function renderAppIcon(
    primaryKey: string,
    displayName: string,
    opts?: { wingetId?: string; dataUrl?: string; sizeClass?: 'sm' | 'md' | 'lg' },
): string {
    const sizeCls = opts?.sizeClass ? `app-icon-${opts.sizeClass}` : 'app-icon-md';
    const builtin =
        lookupBuiltInSvg(primaryKey, displayName) || (opts?.wingetId ? lookupBuiltInSvg(opts.wingetId, displayName) : '');

    if (builtin) {
        return `<div class="app-icon ${sizeCls}" aria-hidden="true">${builtin}</div>`;
    }

    const monogram = escapeAttr(makeMonogram(displayName || primaryKey));
    const bg = monogramColor(opts?.wingetId || primaryKey || displayName);

    if (opts?.dataUrl) {
        return `
            <div class="app-icon ${sizeCls}" style="--mono-bg:${bg}" aria-hidden="true">
                <img class="app-icon-img" src="${escapeAttr(opts.dataUrl)}" alt="" />
                <span class="app-icon-mono" style="display:none">${monogram}</span>
            </div>
        `;
    }

    return `
        <div class="app-icon ${sizeCls} app-icon-fallback" style="--mono-bg:${bg}" aria-hidden="true">
            <span class="app-icon-mono">${monogram}</span>
        </div>
    `;
}
