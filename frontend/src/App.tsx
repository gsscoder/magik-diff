import {useEffect, useRef, useState} from 'react';
import './App.css';
import {
    APIKeyUsedFallback,
    ChangedFiles,
    CommitFileDiff,
    CommitFiles,
    ExplainFile,
    FileDiff,
    GetConfig,
    HasAPIKey,
    RecentCommits,
    SaveConfig,
    SetAPIKey,
} from "../wailsjs/go/main/App";
import {config, gitdiff} from "../wailsjs/go/models";

type RailMode = "changes" | "history";

const COMMIT_PAGE_SIZE = 200;

interface FileStatus {
    glyph: string;
    label: string;
    className: string;
}

// Keys match the lowercase ChangeType values serialized by the Go backend.
const statusStyles: Record<string, FileStatus> = {
    modified: {glyph: "●", label: "Modified", className: "status-modified"},
    added: {glyph: "+", label: "Added", className: "status-added"},
    deleted: {glyph: "−", label: "Deleted", className: "status-deleted"},
    renamed: {glyph: "→", label: "Renamed", className: "status-renamed"},
};

const fallbackStatus: FileStatus = {glyph: "?", label: "Changed", className: ""};

function App() {
    const [mode, setMode] = useState<RailMode>("changes");
    const [files, setFiles] = useState<gitdiff.FileChange[]>([]);
    const [selectedPath, setSelectedPath] = useState<string | null>(null);
    const [diffText, setDiffText] = useState<string>("");

    const [commits, setCommits] = useState<gitdiff.Commit[]>([]);
    const [commitsExhausted, setCommitsExhausted] = useState(false);
    const [selectedCommit, setSelectedCommit] = useState<gitdiff.Commit | null>(null);
    const [commitFiles, setCommitFiles] = useState<gitdiff.FileChange[]>([]);
    const loadingCommits = useRef(false);

    const [cfg, setCfg] = useState<config.Config>(new config.Config());
    const [hasKey, setHasKey] = useState(false);
    const [usedFallback, setUsedFallback] = useState(false);
    const [configDialogOpen, setConfigDialogOpen] = useState(false);

    const [explanation, setExplanation] = useState<string>("");
    const [explaining, setExplaining] = useState(false);
    const [explainError, setExplainError] = useState<string>("");

    const [bannerDismissed, setBannerDismissed] = useState(false);

    const ready = cfg.base_url !== "" && cfg.model !== "" && hasKey;

    useEffect(() => {
        ChangedFiles().then((f) => setFiles(f ?? []));
        checkReadiness();
    }, []);

    function checkReadiness() {
        GetConfig().then(setCfg);
        HasAPIKey().then(setHasKey);
        APIKeyUsedFallback().then(setUsedFallback);
    }

    function selectFile(path: string) {
        setSelectedPath(path);
        setDiffText("");
        if (mode === "history" && selectedCommit) {
            CommitFileDiff(selectedCommit.Hash, path).then(setDiffText);
        } else {
            FileDiff(path).then(setDiffText);
        }
        setExplanation("");
        setExplainError("");
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

    function switchMode(next: RailMode) {
        if (next === mode) {
            return;
        }
        setMode(next);
        setSelectedPath(null);
        setDiffText("");
        setExplanation("");
        setExplainError("");
        setSelectedCommit(null);
        setCommitFiles([]);
        if (next === "history" && commits.length === 0) {
            loadCommits();
        }
    }

    function selectCommit(commit: gitdiff.Commit) {
        setSelectedCommit(commit);
        setSelectedPath(null);
        setDiffText("");
        setExplanation("");
        setExplainError("");
        CommitFiles(commit.Hash).then((f) => setCommitFiles(f ?? []));
    }

    function backToCommits() {
        setSelectedCommit(null);
        setCommitFiles([]);
        setSelectedPath(null);
        setDiffText("");
        setExplanation("");
        setExplainError("");
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
        if (!selectedPath || mode !== "changes") {
            return;
        }
        setExplaining(true);
        setExplanation("");
        setExplainError("");
        ExplainFile(selectedPath)
            .then(setExplanation)
            .catch((err) => setExplainError(String(err)))
            .finally(() => setExplaining(false));
    }

    function openConfigDialog() {
        setConfigDialogOpen(true);
    }

    function closeConfigDialog() {
        setConfigDialogOpen(false);
        checkReadiness();
    }

    function renderFileList(items: gitdiff.FileChange[]) {
        return (
            <ul className="file-list">
                {items.map((file) => {
                    const status = statusStyles[file.Type] ?? fallbackStatus;
                    return (
                        <li
                            key={file.Path}
                            className={file.Path === selectedPath ? "file-item selected" : "file-item"}
                            onClick={() => selectFile(file.Path)}
                        >
                            <span className={`file-type ${status.className}`} title={status.label}>
                                {status.glyph}
                            </span>
                            <span className="file-path">{file.Path}</span>
                        </li>
                    );
                })}
            </ul>
        );
    }

    return (
        <div id="App-root">
            {!ready && !bannerDismissed && (
                <div className="readiness-banner">
                    <span className="readiness-banner-icon">⚠</span>
                    <span className="readiness-banner-text">
                        AI features are disabled.
                        {!hasKey && usedFallback && " (API key loaded from environment variable fallback — OS keyring unavailable.)"}
                    </span>
                    <button className="readiness-banner-action" onClick={openConfigDialog}>Config</button>
                    <button className="readiness-banner-action" onClick={() => setBannerDismissed(true)}>Dismiss</button>
                </div>
            )}
            <div id="App">
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
                    {mode === "changes" && renderFileList(files)}
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
                            <button className="rail-back" onClick={backToCommits}>
                                ← {selectedCommit.Hash.slice(0, 7)} {selectedCommit.Subject}
                            </button>
                            {renderFileList(commitFiles)}
                        </>
                    )}
                </div>
                <div className="diff-pane">
                    <pre>{diffText}</pre>
                </div>
                <div className="explanation-pane">
                    <div className="explanation-header">
                        <button
                            className="explain-button"
                            disabled={!ready || mode !== "changes" || !selectedPath || explaining}
                            title={mode !== "changes" ? "Explain works on working-tree changes only" : undefined}
                            onClick={explain}
                        >
                            {explaining ? "Explaining…" : "Explain"}
                        </button>
                    </div>
                    {!selectedPath && (
                        <p className="placeholder">Select a file to see its explanation</p>
                    )}
                    {selectedPath && mode === "history" && (
                        <p className="placeholder">Explain works on working-tree changes only</p>
                    )}
                    {selectedPath && mode === "changes" && !explaining && !explanation && !explainError && (
                        <p className="placeholder">Click Explain to see an explanation of this diff</p>
                    )}
                    {explaining && (
                        <p className="placeholder">Explaining…</p>
                    )}
                    {explainError && (
                        <p className="explain-error">{explainError}</p>
                    )}
                    {explanation && explanation.split(/\n\n+/).map((paragraph, i) => (
                        <p key={i}>{paragraph}</p>
                    ))}
                </div>
            </div>
            {configDialogOpen && (
                <ConfigDialog
                    initialConfig={cfg}
                    hasKey={hasKey}
                    usedFallback={usedFallback}
                    onClose={closeConfigDialog}
                />
            )}
        </div>
    )
}

function ConfigDialog(props: {
    initialConfig: config.Config;
    hasKey: boolean;
    usedFallback: boolean;
    onClose: () => void;
}) {
    const [baseURL, setBaseURL] = useState(props.initialConfig.base_url);
    const [model, setModel] = useState(props.initialConfig.model);
    const [newKey, setNewKey] = useState("");

    function save() {
        const cfg = new config.Config({base_url: baseURL, model});
        const tasks: Promise<unknown>[] = [SaveConfig(cfg)];
        if (newKey !== "") {
            tasks.push(SetAPIKey(newKey));
        }
        Promise.all(tasks).then(props.onClose);
    }

    return (
        <div className="config-overlay" onClick={props.onClose}>
            <div className="config-dialog" onClick={(e) => e.stopPropagation()}>
                <h2>Explain settings</h2>
                <label className="config-field">
                    Base URL
                    <input
                        type="text"
                        value={baseURL}
                        onChange={(e) => setBaseURL(e.target.value)}
                    />
                </label>
                <label className="config-field">
                    Model
                    <input
                        type="text"
                        value={model}
                        onChange={(e) => setModel(e.target.value)}
                    />
                </label>
                <label className="config-field">
                    {props.hasKey
                        ? props.usedFallback
                            ? "API key: set (via environment fallback)"
                            : "API key: set ✓"
                        : "API key: not set"}
                    <input
                        type="password"
                        placeholder={props.hasKey ? "Enter a new key to replace it" : "Enter API key"}
                        value={newKey}
                        onChange={(e) => setNewKey(e.target.value)}
                    />
                </label>
                <div className="config-dialog-actions">
                    <button onClick={props.onClose}>Cancel</button>
                    <button onClick={save}>Save</button>
                </div>
            </div>
        </div>
    )
}

export default App
