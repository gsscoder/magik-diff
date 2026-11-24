import {useEffect, useRef, useState} from 'react';
import './App.css';
import TitleBar from "./TitleBar";
import './TitleBar.css';
import StatusBar from "./StatusBar";
import './StatusBar.css';
import {
    AcquireProjectBrief,
    APIKeyUsedFallback,
    ChangedFiles,
    CommitFileDiff,
    CommitFiles,
    DeleteAPIKey,
    Explain,
    FileDiff,
    GetConfig,
    HasAPIKey,
    IsGitRepo,
    ListChecks,
    OpenAndSwitchRepo,
    ProjectBrief,
    RecentCommits,
    RunCheck,
    SaveConfig,
    SetAPIKey,
    VerifyLLMConfig,
    Version,
} from "../wailsjs/go/main/App";
import {brief, checks, config, diffparse, gitdiff, main} from "../wailsjs/go/models";
import DiffPane from "./DiffPane";
import ExplanationPane, {CheckState} from "./ExplanationPane";
import FileList from "./FileList";
import ProjectBriefBar from "./ProjectBriefBar";
import {
    EventsOn,
    Position,
    Size,
    WindowFullscreen,
    WindowGetPosition,
    WindowGetSize,
    WindowSetPosition,
    WindowSetSize,
    WindowUnfullscreen,
} from "../wailsjs/runtime/runtime";

type RailMode = "changes" | "history";

type DialogStatus = { kind: "success"; message: string } | { kind: "error"; message: string } | null;

type Provider = "openai" | "azure";

const PROVIDERS: { value: Provider; label: string }[] = [
    {value: "openai", label: "OpenAI-compatible"},
    {value: "azure", label: "Azure Foundry"},
];

const LOCAL_HOSTS = ["localhost", "127.0.0.1", "[::1]"];

function isUnencryptedRemote(baseURL: string): boolean {
    try {
        const url = new URL(baseURL.trim());
        return url.protocol === "http:" && !LOCAL_HOSTS.includes(url.hostname);
    } catch {
        return false;
    }
}

const COMMIT_PAGE_SIZE = 200;

function defaultChecked(files: gitdiff.FileChange[]): Set<string> {
    return new Set(files.filter((f) => f.IsCode).map((f) => f.Path));
}

// Builds a mousedown handler that drag-resizes a pane by listening to
// mousemove/mouseup on window. `direction` controls which way dragging
// grows the pane (+1 when the resizer sits at the pane's leading edge,
// -1 when it sits at the trailing edge); `max` is re-evaluated at
// mousedown time so it can depend on other panes' current widths.
function makeResizeHandler(
    startWidth: number,
    setWidth: (next: number) => void,
    options: { direction: 1 | -1; min: number; max: () => number },
) {
    return (e: React.MouseEvent) => {
        e.preventDefault();
        const startX = e.clientX;
        const onMove = (moveEvent: MouseEvent) => {
            const next = startWidth + options.direction * (moveEvent.clientX - startX);
            const max = options.max();
            setWidth(Math.min(Math.max(next, options.min), Math.max(max, options.min)));
        };
        const onUp = () => {
            window.removeEventListener("mousemove", onMove);
            window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
    };
}

function App() {
    const [mode, setMode] = useState<RailMode>("changes");
    const [files, setFiles] = useState<gitdiff.FileChange[]>([]);
    const [selectedPath, setSelectedPath] = useState<string | null>(null);
    const [parsedDiff, setParsedDiff] = useState<diffparse.FileDiff | null>(null);

    const [commits, setCommits] = useState<gitdiff.Commit[]>([]);
    const [commitsExhausted, setCommitsExhausted] = useState(false);
    const [selectedCommit, setSelectedCommit] = useState<gitdiff.Commit | null>(null);
    const [commitFiles, setCommitFiles] = useState<gitdiff.FileChange[]>([]);
    const loadingCommits = useRef(false);

    const [checked, setChecked] = useState<Set<string>>(new Set());
    const [fileFilter, setFileFilter] = useState("");

    const [cfg, setCfg] = useState<config.Config>(new config.Config());
    const [hasKey, setHasKey] = useState(false);
    const [usedFallback, setUsedFallback] = useState(false);
    const [llmConfigDialogOpen, setLlmConfigDialogOpen] = useState(false);
    const [aboutDialogOpen, setAboutDialogOpen] = useState(false);

    const [explanation, setExplanation] = useState<string>("");
    const [streamingText, setStreamingText] = useState<string>("");
    const [explaining, setExplaining] = useState(false);
    const [explainError, setExplainError] = useState<string>("");
    const [diffError, setDiffError] = useState<string>("");
    const [explainedCount, setExplainedCount] = useState(0);
    const [explanationExpanded, setExplanationExpanded] = useState(false);
    const [explainWidth, setExplainWidth] = useState(380);
    const [railWidth, setRailWidth] = useState(240);

    const [projectBrief, setProjectBrief] = useState<brief.State | null>(null);
    const [acquiringBrief, setAcquiringBrief] = useState(false);
    const [briefError, setBriefError] = useState<string>("");
    const [briefExpanded, setBriefExpanded] = useState(false);
    const [useBrief, setUseBrief] = useState(true);

    const startResize = makeResizeHandler(explainWidth, setExplainWidth, {
        direction: -1,
        min: 240,
        max: () => window.innerWidth - 240 - 200,
    });

    const startRailResize = makeResizeHandler(railWidth, setRailWidth, {
        direction: 1,
        min: 160,
        max: () => window.innerWidth - explainWidth - 300,
    });

    const [checksList, setChecksList] = useState<checks.Check[]>([]);
    const [checkResults, setCheckResults] = useState<Record<string, CheckState>>({});

    const [bannerDismissed, setBannerDismissed] = useState(false);

    const [repoValid, setRepoValid] = useState<boolean | null>(null);
    const [openError, setOpenError] = useState<string>("");
    const [repoGeneration, setRepoGeneration] = useState(0);

    const [zoom, setZoom] = useState(() => Number(localStorage.getItem("mdiff-zoom")) || 1);

    const [zen, setZen] = useState(false);
    const [railVisible, setRailVisible] = useState(true);
    const zenRef = useRef(zen);
    zenRef.current = zen;
    const preZenBounds = useRef<{ size: Size; position: Position } | null>(null);

    // Refs mirroring state consumed by the repo:changed handler below, kept
    // current each render so the handler (subscribed once on mount) always
    // sees the latest values without adding them to its dependency array.
    const modeRef = useRef(mode);
    modeRef.current = mode;
    const selectedPathRef = useRef(selectedPath);
    selectedPathRef.current = selectedPath;
    const selectedCommitRef = useRef(selectedCommit);
    selectedCommitRef.current = selectedCommit;
    const repoGenerationRef = useRef(repoGeneration);
    repoGenerationRef.current = repoGeneration;

    const [splitView, setSplitView] = useState<boolean>(() => {
        try {
            return localStorage.getItem("magikdiff.splitView") === "1";
        } catch {
            return false;
        }
    });

    const ready = cfg.base_url !== "" && cfg.model !== "" && (cfg.provider !== "azure" || hasKey);

    useEffect(() => {
        IsGitRepo()
            .then((ok) => {
                setRepoValid(ok);
                if (ok) {
                    loadRepoData();
                }
            })
            .catch((err) => {
                setRepoValid(false);
                setOpenError(String(err));
            });
    }, []);

    // Background refresh: the backend watches the repo and emits this event
    // when tracked state changes outside the running app (an agent, another
    // terminal, ...). Unlike loadRepoData/handleOpenFolder, this must never
    // clobber the user's checkbox selection, selected file, or explanation panel.
    useEffect(() => {
        return EventsOn("repo:changed", handleRepoChanged);
    }, []);

    // Live explanation: the backend emits one event per chunk of LLM prose,
    // so the pane fills in as it is generated instead of sitting on a
    // placeholder for what can be a minute. The promise's final text still
    // wins; this buffer only covers the wait.
    useEffect(() => {
        return EventsOn("explain:delta", (chunk) => {
            if (typeof chunk === "string") {
                setStreamingText((prev) => prev + chunk);
            }
        });
    }, []);

    function handleRepoChanged() {
        ChangedFiles()
            .then((f) => {
                const loaded = f ?? [];
                setFiles(loaded);
                setOpenError("");

                if (modeRef.current !== "changes") {
                    return;
                }

                const paths = new Set(loaded.map((file) => file.Path));
                setChecked((prev) => new Set([...prev].filter((p) => paths.has(p))));

                const current = selectedPathRef.current;
                if (current === null) {
                    return;
                }
                if (!paths.has(current)) {
                    setSelectedPath(null);
                    setParsedDiff(null);
                    return;
                }
                FileDiff(current)
                    .then((diff) => {
                        if (selectedPathRef.current !== current) {
                            return;
                        }
                        setParsedDiff(diff);
                    })
                    .catch((err) => {
                        if (selectedPathRef.current !== current) {
                            return;
                        }
                        setDiffError(String(err));
                    });
            })
            .catch((err) => setOpenError(String(err)));
    }

    useEffect(() => {
        function handleKeyDown(e: KeyboardEvent) {
            if (e.key === "F11") {
                e.preventDefault();
                toggleZen();
                return;
            }
            if (!(e.ctrlKey || e.metaKey)) {
                return;
            }
            if (e.key === "=" || e.key === "+") {
                e.preventDefault();
                setZoom((z) => Math.min(1.8, Math.max(0.7, z + 0.1)));
            } else if (e.key === "-") {
                e.preventDefault();
                setZoom((z) => Math.min(1.8, Math.max(0.7, z - 0.1)));
            } else if (e.key === "0") {
                e.preventDefault();
                setZoom(1);
            } else if (e.key === "m" || e.key === "M") {
                e.preventDefault();
                openLlmConfigDialog();
            } else if (e.key === "b" || e.key === "B") {
                e.preventDefault();
                setRailVisible((v) => !v);
            }
        }
        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    }, []);

    useEffect(() => {
        localStorage.setItem("mdiff-zoom", String(zoom));
    }, [zoom]);

    useEffect(() => {
        try {
            localStorage.setItem("magikdiff.splitView", splitView ? "1" : "0");
        } catch {
            // localStorage can throw in some contexts (e.g. private browsing)
        }
    }, [splitView]);

    function toggleZen() {
        if (zenRef.current) {
            WindowUnfullscreen();
            const bounds = preZenBounds.current;
            if (bounds) {
                WindowSetSize(bounds.size.w, bounds.size.h);
                WindowSetPosition(bounds.position.x, bounds.position.y);
            }
            setZen(false);
            return;
        }
        Promise.all([WindowGetSize(), WindowGetPosition()]).then(([size, position]) => {
            preZenBounds.current = {size, position};
            WindowFullscreen();
        });
        setZen(true);
    }

    function checkReadiness() {
        GetConfig().then(setCfg);
        HasAPIKey().then(setHasKey);
        APIKeyUsedFallback().then(setUsedFallback);
    }

    function loadRepoData() {
        ChangedFiles()
            .then((f) => {
                const loaded = f ?? [];
                setFiles(loaded);
                setChecked(defaultChecked(loaded));
                setOpenError("");
            })
            .catch((err) => setOpenError(String(err)));
        checkReadiness();
        ListChecks().then((c) => setChecksList(c ?? []));
        ProjectBrief().then(setProjectBrief);
    }

    function selectFile(path: string) {
        setSelectedPath(path);
        setParsedDiff(null);
        setDiffError("");
        const request = mode === "history" && selectedCommit
            ? CommitFileDiff(selectedCommit.Hash, path)
            : FileDiff(path);
        request
            .then((diff) => {
                if (selectedPathRef.current !== path) {
                    return;
                }
                setParsedDiff(diff);
            })
            .catch((err) => {
                if (selectedPathRef.current !== path) {
                    return;
                }
                setDiffError(String(err));
            });
    }

    function loadCommits() {
        if (loadingCommits.current || commitsExhausted) {
            return;
        }
        loadingCommits.current = true;
        RecentCommits(commits.length, COMMIT_PAGE_SIZE)
            .then((batch) => {
                const page = batch ?? [];
                setCommits((prev) => [...prev, ...page]);
                if (page.length < COMMIT_PAGE_SIZE) {
                    setCommitsExhausted(true);
                }
            })
            .finally(() => {
                loadingCommits.current = false;
            });
    }

    // Common reset shared by every action that abandons the current
    // file/commit selection: clears the diff and explanation state that
    // would otherwise keep referring to something no longer selected.
    function resetSelection() {
        setSelectedPath(null);
        setParsedDiff(null);
        setExplanation("");
        setExplainError("");
        setDiffError("");
        setCheckResults({});
    }

    function switchMode(next: RailMode) {
        if (next === mode) {
            return;
        }
        setMode(next);
        resetSelection();
        setSelectedCommit(null);
        setCommitFiles([]);
        setChecked(new Set());
        setFileFilter("");
        if (next === "history" && commits.length === 0) {
            loadCommits();
        }
    }

    function selectCommit(commit: gitdiff.Commit) {
        setSelectedCommit(commit);
        resetSelection();
        setFileFilter("");
        const hash = commit.Hash;
        CommitFiles(hash)
            .then((f) => {
                if (selectedCommitRef.current?.Hash !== hash) {
                    return;
                }
                const loaded = f ?? [];
                setCommitFiles(loaded);
                setChecked(defaultChecked(loaded));
            })
            .catch((err) => {
                if (selectedCommitRef.current?.Hash !== hash) {
                    return;
                }
                setDiffError(String(err));
            });
    }

    function backToCommits() {
        setSelectedCommit(null);
        setCommitFiles([]);
        resetSelection();
        setChecked(new Set());
        setFileFilter("");
    }

    function handleRailScroll(e: React.UIEvent<HTMLDivElement>) {
        if (mode !== "history" || selectedCommit) {
            return;
        }
        const el = e.currentTarget;
        if (el.scrollTop + el.clientHeight >= el.scrollHeight - 20) {
            loadCommits();
        }
    }

    function explain() {
        if (checked.size === 0) {
            return;
        }
        setExplaining(true);
        setExplanation("");
        setStreamingText("");
        setExplainError("");
        setExplainedCount(checked.size);
        const effectiveUseBrief = (projectBrief?.Stored ?? false) && useBrief;
        const request = mode === "history" && selectedCommit
            ? Explain(selectedCommit.Hash, [...checked], effectiveUseBrief)
            : Explain("", [...checked], effectiveUseBrief);
        // The streamed buffer is dropped in the same update that stores the
        // authoritative text, so the prose never renders twice for a frame.
        request
            .then((text) => {
                setExplanation(text);
                setStreamingText("");
            })
            .catch((err) => {
                setExplainError(String(err));
                setStreamingText("");
            })
            .finally(() => setExplaining(false));
    }

    function acquireProjectBrief() {
        setAcquiringBrief(true);
        setBriefError("");
        const generation = repoGenerationRef.current;
        AcquireProjectBrief()
            .then((state) => {
                if (repoGenerationRef.current !== generation) {
                    return;
                }
                setProjectBrief(state);
            })
            .catch((err) => {
                if (repoGenerationRef.current !== generation) {
                    return;
                }
                setBriefError(String(err));
            })
            .finally(() => setAcquiringBrief(false));
    }

    function runCheck(check: checks.Check) {
        if (checked.size === 0) {
            return;
        }
        setCheckResults((prev) => ({...prev, [check.Name]: {kind: "running"}}));
        const request = mode === "history" && selectedCommit
            ? RunCheck(selectedCommit.Hash, check.Name, [...checked])
            : RunCheck("", check.Name, [...checked]);
        request
            .then((result) => setCheckResults((prev) => ({...prev, [check.Name]: {kind: "done", result}})))
            .catch((err) => setCheckResults((prev) => ({...prev, [check.Name]: {kind: "error", message: String(err)}})));
    }

    function openLlmConfigDialog() {
        setLlmConfigDialogOpen(true);
    }

    function closeLlmConfigDialog() {
        setLlmConfigDialogOpen(false);
        checkReadiness();
    }

    function openAboutDialog() {
        setAboutDialogOpen(true);
    }

    function closeAboutDialog() {
        setAboutDialogOpen(false);
    }

    function handleOpenFolder() {
        OpenAndSwitchRepo().then((res: main.OpenFolderResult) => {
            if (res.Canceled) {
                return;
            }
            if (!res.Valid) {
                setOpenError(`"${res.Path}" is not a git repository.`);
                return;
            }
            setOpenError("");
            setRepoValid(true);
            setRepoGeneration((g) => g + 1);
            // The previous repo's history is irrelevant once cwd switches:
            // its commit hashes don't exist in the new repo, so every click
            // would silently produce an empty result forever.
            setMode("changes");
            setSelectedCommit(null);
            setCommitFiles([]);
            setCommits([]);
            setCommitsExhausted(false);
            resetSelection();
            setChecked(new Set());
            setProjectBrief(null);
            setBriefExpanded(false);
            loadRepoData();
        });
    }

    function toggleChecked(path: string) {
        setChecked((prev) => {
            const next = new Set(prev);
            if (next.has(path)) {
                next.delete(path);
            } else {
                next.add(path);
            }
            return next;
        });
    }

    function toggleCheckedAll(items: gitdiff.FileChange[]) {
        const allChecked = items.length > 0 && items.every((f) => checked.has(f.Path));
        setChecked((prev) => {
            const next = new Set(prev);
            for (const f of items) {
                if (allChecked) {
                    next.delete(f.Path);
                } else {
                    next.add(f.Path);
                }
            }
            return next;
        });
    }

    if (repoValid === null) {
        return null;
    }

    if (!repoValid) {
        return (
            <div id="App-shell">
                {!zen && <TitleBar onOpenRepo={handleOpenFolder} onOpenLlmConfig={openLlmConfigDialog} onOpenAbout={openAboutDialog} railVisible={railVisible} onToggleRailVisible={() => setRailVisible((v) => !v)}/>}
                <div className="readiness-banner">
                    <span className="readiness-banner-icon">⚠</span>
                    <span className="readiness-banner-text">
                        This folder is not a git repository.
                        {openError && ` ${openError}`}
                    </span>
                    <button className="readiness-banner-action" onClick={handleOpenFolder}>Open</button>
                </div>
                {!zen && <StatusBar key={repoGeneration}/>}
                {aboutDialogOpen && <AboutDialog onClose={closeAboutDialog} />}
            </div>
        );
    }

    return (
        <div id="App-shell">
            {/* TitleBar sits outside the zoomed #App-root: it's OS chrome, not zoomable content */}
            {!zen && <TitleBar onOpenRepo={handleOpenFolder} onOpenLlmConfig={openLlmConfigDialog} onOpenAbout={openAboutDialog} railVisible={railVisible} onToggleRailVisible={() => setRailVisible((v) => !v)}/>}
            <div id="App-root" style={{zoom}}>
                {openError && (
                    <div className="readiness-banner">
                        <span className="readiness-banner-icon">⚠</span>
                        <span className="readiness-banner-text">{openError}</span>
                        <button className="readiness-banner-action" onClick={() => setOpenError("")}>Dismiss</button>
                    </div>
                )}
                {!ready && !bannerDismissed && (
                    <div className="readiness-banner">
                        <span className="readiness-banner-icon">⚠</span>
                        <span className="readiness-banner-text">
                            AI features are disabled.
                            {hasKey && usedFallback && " (API key loaded from environment variable fallback — OS keyring unavailable.)"}
                            {!hasKey && usedFallback && " (OS keyring unavailable and no API key found in the environment.)"}
                        </span>
                        <button className="readiness-banner-action" onClick={openLlmConfigDialog}>Config</button>
                        <button className="readiness-banner-action" onClick={() => setBannerDismissed(true)}>Dismiss</button>
                    </div>
                )}
                <div
                    id="App"
                    className={[explanationExpanded && "explanation-expanded", !railVisible && "rail-collapsed"].filter(Boolean).join(" ") || undefined}
                    style={{"--explain-width": `${explainWidth}px`, "--rail-width": `${railWidth}px`} as React.CSSProperties}
                >
                    {railVisible && (
                        <>
                            <div className="rail" onScroll={handleRailScroll}>
                                <div className="rail-tabs">
                                    <button
                                        className={mode === "changes" ? "rail-tab selected" : "rail-tab"}
                                        onClick={() => switchMode("changes")}
                                    >
                                        Changes
                                    </button>
                                    <button
                                        className={mode === "history" ? "rail-tab selected" : "rail-tab"}
                                        onClick={() => switchMode("history")}
                                    >
                                        History
                                    </button>
                                </div>
                                {mode === "changes" && (
                                    <FileList
                                        items={files}
                                        selectedPath={selectedPath}
                                        checked={checked}
                                        fileFilter={fileFilter}
                                        onFileFilterChange={setFileFilter}
                                        onSelectFile={selectFile}
                                        onToggleChecked={toggleChecked}
                                        onToggleCheckedAll={toggleCheckedAll}
                                        historyMode={false}
                                    />
                                )}
                                {mode === "history" && !selectedCommit && (
                                    <ul className="file-list">
                                        {commits.map((commit) => (
                                            <li
                                                key={commit.Hash}
                                                className="commit-item"
                                                onClick={() => selectCommit(commit)}
                                            >
                                                <span className="commit-subject">{commit.Subject}</span>
                                                <span className="commit-meta">
                                                    {commit.Author} · {commit.Date} · {commit.Hash.slice(0, 7)}
                                                </span>
                                            </li>
                                        ))}
                                    </ul>
                                )}
                                {mode === "history" && selectedCommit && (
                                    <>
                                        <div className="rail-back">
                                            <button className="rail-back-arrow" onClick={backToCommits} aria-label="Back to commit list">←</button>
                                            <span className="rail-back-label">{selectedCommit.Hash.slice(0, 7)} {selectedCommit.Subject}</span>
                                        </div>
                                        <FileList
                                            items={commitFiles}
                                            selectedPath={selectedPath}
                                            checked={checked}
                                            fileFilter={fileFilter}
                                            onFileFilterChange={setFileFilter}
                                            onSelectFile={selectFile}
                                            onToggleChecked={toggleChecked}
                                            onToggleCheckedAll={toggleCheckedAll}
                                            historyMode={true}
                                        />
                                    </>
                                )}
                            </div>
                            <div className="pane-resizer rail-resizer" onMouseDown={startRailResize} />
                        </>
                    )}
                    <div className="diff-column">
                        <ProjectBriefBar
                            state={projectBrief}
                            acquiring={acquiringBrief}
                            error={briefError}
                            onAcquire={acquireProjectBrief}
                            expanded={briefExpanded}
                            onExpandedChange={setBriefExpanded}
                            useBrief={useBrief}
                            onUseBriefChange={setUseBrief}
                        />
                        <DiffPane
                            parsedDiff={parsedDiff}
                            diffError={diffError}
                            splitView={splitView}
                            onSplitViewChange={setSplitView}
                        />
                    </div>
                    {!explanationExpanded && (
                        <div className="pane-resizer" onMouseDown={startResize} />
                    )}
                    <ExplanationPane
                        explanation={explanation}
                        streamingText={streamingText}
                        explaining={explaining}
                        explainError={explainError}
                        explainedCount={explainedCount}
                        explanationExpanded={explanationExpanded}
                        onExplanationExpandedChange={setExplanationExpanded}
                        onExplain={explain}
                        ready={ready}
                        checkedCount={checked.size}
                        checksList={checksList}
                        checkResults={checkResults}
                        onRunCheck={runCheck}
                    />
                </div>
                {llmConfigDialogOpen && (
                    <LlmConfigDialog
                        initialConfig={cfg}
                        hasKey={hasKey}
                        usedFallback={usedFallback}
                        onClose={closeLlmConfigDialog}
                    />
                )}
                {aboutDialogOpen && <AboutDialog onClose={closeAboutDialog} />}
            </div>
            {!zen && <StatusBar key={repoGeneration}/>}
        </div>
    )
}

function LlmConfigDialog(props: {
    initialConfig: config.Config;
    hasKey: boolean;
    usedFallback: boolean;
    onClose: () => void;
}) {
    const initialProvider: Provider = props.initialConfig.provider === "azure" ? "azure" : "openai";
    const [provider, setProvider] = useState<Provider>(initialProvider);
    const [baseURL, setBaseURL] = useState(props.initialConfig.base_url);
    const [model, setModel] = useState(props.initialConfig.model);
    const [newKey, setNewKey] = useState("");
    const [removeKeyPending, setRemoveKeyPending] = useState(false);
    const [busy, setBusy] = useState<"verify" | "save" | null>(null);
    const [status, setStatus] = useState<DialogStatus>(null);

    const azure = provider === "azure";
    const typedKey = newKey.trim();
    const cleanBaseURL = baseURL.trim();
    const cleanModel = model.trim();
    const keyAvailable = props.hasKey && !removeKeyPending;
    const connectionIncomplete = cleanBaseURL === "" || cleanModel === "" || (azure && !keyAvailable && typedKey === "");
    const dirty =
        provider !== initialProvider ||
        cleanBaseURL !== props.initialConfig.base_url ||
        cleanModel !== props.initialConfig.model ||
        typedKey !== "" ||
        removeKeyPending;

    useEffect(() => {
        if (busy !== null) {
            return;
        }
        function onKeyDown(e: KeyboardEvent) {
            if (e.key === "Escape") {
                props.onClose();
            }
        }
        window.addEventListener("keydown", onKeyDown);
        return () => window.removeEventListener("keydown", onKeyDown);
    }, [busy, props.onClose]);

    function currentConfig() {
        return new config.Config({provider, base_url: cleanBaseURL, model: cleanModel});
    }

    function save() {
        setBusy("save");
        setStatus(null);
        const tasks: Promise<unknown>[] = [SaveConfig(currentConfig())];
        if (typedKey !== "") {
            tasks.push(SetAPIKey(typedKey));
        }
        Promise.all(tasks)
            .then(() => (removeKeyPending ? DeleteAPIKey() : undefined))
            .then(props.onClose)
            .catch((err) => setStatus({kind: "error", message: String(err)}))
            .finally(() => setBusy(null));
    }

    function verify() {
        setBusy("verify");
        setStatus(null);
        const startedAt = performance.now();
        VerifyLLMConfig(currentConfig(), typedKey)
            .then(() => {
                const seconds = ((performance.now() - startedAt) / 1000).toFixed(1);
                setStatus({kind: "success", message: `✓ Connected: ${cleanModel} answered in ${seconds} s`});
            })
            .catch((err) => setStatus({kind: "error", message: String(err)}))
            .finally(() => setBusy(null));
    }

    return (
        <div className="config-overlay">
            <form
                className="config-dialog"
                onClick={(e) => e.stopPropagation()}
                onSubmit={(e) => {
                    e.preventDefault();
                    save();
                }}
            >
                <h2>LLM config</h2>
                <p className="config-subtitle">
                    Works with any OpenAI-compatible API: OpenAI, OpenRouter, Ollama, LM Studio, Azure.
                </p>
                <div className="view-toggle config-provider" role="group" aria-label="Connection mode">
                    {PROVIDERS.map(({value, label}) => (
                        <button
                            key={value}
                            type="button"
                            className={provider === value ? "active" : undefined}
                            aria-pressed={provider === value}
                            disabled={busy !== null}
                            onClick={() => setProvider(value)}
                        >
                            {label}
                        </button>
                    ))}
                </div>
                <label className="config-field">
                    Base URL
                    <input
                        type="text"
                        placeholder={azure ? "https://<resource>.cognitiveservices.azure.com" : "https://api.openai.com/v1"}
                        value={baseURL}
                        disabled={busy !== null}
                        onChange={(e) => setBaseURL(e.target.value)}
                    />
                    {azure && (
                        <span className="config-hint">
                            Resource endpoint. The full Target URI from the Azure portal also works.
                        </span>
                    )}
                </label>
                {isUnencryptedRemote(baseURL) && (
                    <p className="config-warning">
                        Warning: this URL uses http:// on a remote host. The API key and your code structure would travel unencrypted.
                    </p>
                )}
                <label className="config-field">
                    {azure ? "Deployment name" : "Model"}
                    <input
                        type="text"
                        value={model}
                        disabled={busy !== null}
                        onChange={(e) => setModel(e.target.value)}
                    />
                </label>
                <div className="config-field">
                    <label htmlFor="config-api-key">
                        {removeKeyPending
                            ? "API key: will be removed on Save"
                            : props.hasKey
                                ? props.usedFallback
                                    ? "API key: set (via environment fallback)"
                                    : "API key: set ✓"
                                : "API key: not set"}
                    </label>
                    <div className="config-key-row">
                        <input
                            id="config-api-key"
                            type="password"
                            autoComplete="off"
                            placeholder={props.hasKey ? "Enter a new key to replace it" : "Enter API key"}
                            value={newKey}
                            disabled={busy !== null}
                            onChange={(e) => {
                                setNewKey(e.target.value);
                                if (e.target.value.trim() !== "") {
                                    setRemoveKeyPending(false);
                                }
                            }}
                        />
                        {props.hasKey && !props.usedFallback && (
                            <button type="button" disabled={busy !== null} onClick={() => setRemoveKeyPending(!removeKeyPending)}>
                                {removeKeyPending ? "Undo" : "Remove key"}
                            </button>
                        )}
                    </div>
                    <span className="config-hint">
                        {azure
                            ? "Required. Saved in the OS keyring."
                            : "Saved in the OS keyring. Leave empty for local servers without a key."}
                    </span>
                </div>
                <div className="config-dialog-actions">
                    <button type="button" disabled={busy !== null || connectionIncomplete} onClick={verify}>
                        {busy === "verify" ? "Verifying…" : "Verify"}
                    </button>
                    <button type="button" disabled={busy !== null} onClick={props.onClose}>Cancel</button>
                    <button type="submit" disabled={busy !== null || !dirty || connectionIncomplete}>Save</button>
                </div>
                {status?.kind === "success" && (
                    <p className="config-verify-success" role="status">{status.message}</p>
                )}
                {status?.kind === "error" && (
                    <p className="explain-error" role="alert">{status.message}</p>
                )}
            </form>
        </div>
    )
}

function AboutDialog(props: { onClose: () => void }) {
    const [version, setVersion] = useState("");

    useEffect(() => {
        Version().then(setVersion);
    }, []);

    return (
        <div className="config-overlay" onClick={props.onClose}>
            <div className="config-dialog about-dialog" onClick={(e) => e.stopPropagation()}>
                <h2>Magik Diff</h2>
                <div className="about-meta">
                    <div><span>Version</span><span>{version}</span></div>
                    <div><span>Author</span><span>koder0x</span></div>
                    <div><span>License</span><span>MIT</span></div>
                    <div><span>Stack</span><span>Go, Wails, React, TypeScript</span></div>
                </div>
                <hr className="about-divider" />
                <div className="about-shortcuts">
                    <div><span>F11</span><span>Toggle zen mode</span></div>
                    <div><span>Ctrl+B</span><span>Show/hide Changes/History pane</span></div>
                    <div><span>Ctrl+M</span><span>Open LLM config</span></div>
                    <div><span>Ctrl+ +/-</span><span>Zoom in / out</span></div>
                    <div><span>Ctrl+0</span><span>Reset zoom</span></div>
                    <div><span>Esc</span><span>Close open menu</span></div>
                </div>
                <div className="config-dialog-actions">
                    <button onClick={props.onClose}>Close</button>
                </div>
            </div>
        </div>
    );
}

export default App
