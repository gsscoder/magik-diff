import {useEffect, useRef, useState} from 'react';
import ReactMarkdown from 'react-markdown';
import './App.css';
import TitleBar from "./TitleBar";
import './TitleBar.css';
import {
    APIKeyUsedFallback,
    ChangedFiles,
    CommitFileDiff,
    CommitFiles,
    Explain,
    FileDiff,
    GetAPIKey,
    GetConfig,
    HasAPIKey,
    IsGitRepo,
    ListChecks,
    OpenAndSwitchRepo,
    RecentCommits,
    RunCheck,
    SaveConfig,
    SetAPIKey,
    VerifyLLMConfig,
} from "../wailsjs/go/main/App";
import {checks, config, diffparse, gitdiff, main} from "../wailsjs/go/models";
import {splitRows} from "./splitRows";

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

function defaultChecked(files: gitdiff.FileChange[]): Set<string> {
    return new Set(files.filter((f) => f.IsCode).map((f) => f.Path));
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

    const [cfg, setCfg] = useState<config.Config>(new config.Config());
    const [hasKey, setHasKey] = useState(false);
    const [usedFallback, setUsedFallback] = useState(false);
    const [configDialogOpen, setConfigDialogOpen] = useState(false);

    const [explanation, setExplanation] = useState<string>("");
    const [explaining, setExplaining] = useState(false);
    const [explainError, setExplainError] = useState<string>("");
    const [explainedCount, setExplainedCount] = useState(0);
    const [explanationExpanded, setExplanationExpanded] = useState(false);
    const [explainWidth, setExplainWidth] = useState(380);
    const [railWidth, setRailWidth] = useState(240);

    const startResize = (e: React.MouseEvent) => {
        e.preventDefault();
        const startX = e.clientX;
        const startWidth = explainWidth;
        const onMove = (moveEvent: MouseEvent) => {
            const next = startWidth - (moveEvent.clientX - startX);
            const max = window.innerWidth - 240 - 200;
            setExplainWidth(Math.min(Math.max(next, 240), Math.max(max, 240)));
        };
        const onUp = () => {
            window.removeEventListener("mousemove", onMove);
            window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
    };

    const startRailResize = (e: React.MouseEvent) => {
        e.preventDefault();
        const startX = e.clientX;
        const startWidth = railWidth;
        const onMove = (moveEvent: MouseEvent) => {
            const next = startWidth + (moveEvent.clientX - startX);
            const max = window.innerWidth - explainWidth - 300;
            setRailWidth(Math.min(Math.max(next, 160), Math.max(max, 160)));
        };
        const onUp = () => {
            window.removeEventListener("mousemove", onMove);
            window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
    };

    const [checksList, setChecksList] = useState<checks.Check[]>([]);
    const [checkResults, setCheckResults] = useState<Record<string, { running: boolean; result: string; error: string }>>({});

    const [bannerDismissed, setBannerDismissed] = useState(false);

    const [repoValid, setRepoValid] = useState<boolean | null>(null);
    const [openError, setOpenError] = useState<string>("");
    const [repoGeneration, setRepoGeneration] = useState(0);

    const [zoom, setZoom] = useState(() => Number(localStorage.getItem("mdiff-zoom")) || 1);

    const [splitView, setSplitView] = useState<boolean>(() => {
        try {
            return localStorage.getItem("magikdiff.splitView") === "1";
        } catch {
            return false;
        }
    });

    const ready = cfg.base_url !== "" && cfg.model !== "" && hasKey;

    useEffect(() => {
        IsGitRepo().then((ok) => {
            setRepoValid(ok);
            if (ok) {
                loadRepoData();
            }
        });
    }, []);

    useEffect(() => {
        function handleKeyDown(e: KeyboardEvent) {
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
            } else if (e.key === "l" || e.key === "L") {
                e.preventDefault();
                openConfigDialog();
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

    function checkReadiness() {
        GetConfig().then(setCfg);
        HasAPIKey().then(setHasKey);
        APIKeyUsedFallback().then(setUsedFallback);
    }

    function loadRepoData() {
        ChangedFiles().then((f) => {
            const loaded = f ?? [];
            setFiles(loaded);
            setChecked(defaultChecked(loaded));
        });
        checkReadiness();
        ListChecks().then((c) => setChecksList(c ?? []));
    }

    function selectFile(path: string) {
        setSelectedPath(path);
        setParsedDiff(null);
        if (mode === "history" && selectedCommit) {
            CommitFileDiff(selectedCommit.Hash, path).then(setParsedDiff);
        } else {
            FileDiff(path).then(setParsedDiff);
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
        setParsedDiff(null);
        setExplanation("");
        setExplainError("");
        setCheckResults({});
        setSelectedCommit(null);
        setCommitFiles([]);
        setChecked(new Set());
        if (next === "history" && commits.length === 0) {
            loadCommits();
        }
    }

    function selectCommit(commit: gitdiff.Commit) {
        setSelectedCommit(commit);
        setSelectedPath(null);
        setParsedDiff(null);
        setExplanation("");
        setExplainError("");
        setCheckResults({});
        CommitFiles(commit.Hash).then((f) => {
            const loaded = f ?? [];
            setCommitFiles(loaded);
            setChecked(defaultChecked(loaded));
        });
    }

    function backToCommits() {
        setSelectedCommit(null);
        setCommitFiles([]);
        setSelectedPath(null);
        setParsedDiff(null);
        setExplanation("");
        setExplainError("");
        setCheckResults({});
        setChecked(new Set());
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
        setExplainError("");
        setExplainedCount(checked.size);
        const request = mode === "history" && selectedCommit
            ? Explain(selectedCommit.Hash, [...checked])
            : Explain("", [...checked]);
        request
            .then(setExplanation)
            .catch((err) => setExplainError(String(err)))
            .finally(() => setExplaining(false));
    }

    function runCheck(check: checks.Check) {
        if (checked.size === 0) {
            return;
        }
        setCheckResults((prev) => ({...prev, [check.Name]: {running: true, result: "", error: ""}}));
        const request = mode === "history" && selectedCommit
            ? RunCheck(selectedCommit.Hash, check.Name, [...checked])
            : RunCheck("", check.Name, [...checked]);
        request
            .then((result) => setCheckResults((prev) => ({...prev, [check.Name]: {running: false, result, error: ""}})))
            .catch((err) => setCheckResults((prev) => ({...prev, [check.Name]: {running: false, result: "", error: String(err)}})));
    }

    function openConfigDialog() {
        setConfigDialogOpen(true);
    }

    function closeConfigDialog() {
        setConfigDialogOpen(false);
        checkReadiness();
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
            loadRepoData();
        });
    }

    function renderLineContent(line: diffparse.Line) {
        const hl = line.Highlight;
        if (hl.End <= hl.Start) {
            return line.Content;
        }
        const chars = [...line.Content];
        return (
            <>
                {chars.slice(0, hl.Start).join("")}
                <span className="diff-hl">{chars.slice(hl.Start, hl.End).join("")}</span>
                {chars.slice(hl.End).join("")}
            </>
        );
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
        setChecked(allChecked ? new Set() : new Set(items.map((f) => f.Path)));
    }

    function renderFileList(items: gitdiff.FileChange[]) {
        const allChecked = items.length > 0 && items.every((f) => checked.has(f.Path));
        const someChecked = items.some((f) => checked.has(f.Path));
        return (
            <>
                <div className="file-list-select-all">
                    <input
                        type="checkbox"
                        checked={allChecked}
                        ref={(el) => {
                            if (el) {
                                el.indeterminate = !allChecked && someChecked;
                            }
                        }}
                        onChange={() => toggleCheckedAll(items)}
                    />
                    <span>Select all</span>
                </div>
                <ul className="file-list">
                    {items.map((file) => {
                        const status = statusStyles[file.Type] ?? fallbackStatus;
                        return (
                            <li
                                key={file.Path}
                                className={file.Path === selectedPath ? "file-item selected" : "file-item"}
                                onClick={() => selectFile(file.Path)}
                            >
                                <input
                                    type="checkbox"
                                    className="file-checkbox"
                                    checked={checked.has(file.Path)}
                                    onClick={(e) => e.stopPropagation()}
                                    onChange={() => toggleChecked(file.Path)}
                                />
                                <span className={`file-type ${status.className}`} title={status.label}>
                                    {status.glyph}
                                </span>
                                <span className="file-path">{file.Path}</span>
                            </li>
                        );
                    })}
                </ul>
            </>
        );
    }

    if (repoValid === null) {
        return null;
    }

    if (!repoValid) {
        return (
            <div id="App-shell">
                <TitleBar key={repoGeneration}/>
                <div className="readiness-banner">
                    <span className="readiness-banner-icon">⚠</span>
                    <span className="readiness-banner-text">
                        This folder is not a git repository.
                        {openError && ` ${openError}`}
                    </span>
                    <button className="readiness-banner-action" onClick={handleOpenFolder}>Open</button>
                </div>
            </div>
        );
    }

    return (
        <div id="App-shell">
            {/* TitleBar sits outside the zoomed #App-root: it's OS chrome, not zoomable content */}
            <TitleBar key={repoGeneration}/>
            <div id="App-root" style={{zoom}}>
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
                <div
                    id="App"
                    className={explanationExpanded ? "explanation-expanded" : undefined}
                    style={{"--explain-width": `${explainWidth}px`, "--rail-width": `${railWidth}px`} as React.CSSProperties}
                >
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
                    <div className="pane-resizer rail-resizer" onMouseDown={startRailResize} />
                    <div className="diff-pane">
                        {!parsedDiff && (
                            <p className="placeholder">Select a file to see its diff</p>
                        )}
                        {parsedDiff && parsedDiff.Path !== "" && (
                            <div className="diff-file-header">
                                <span className="diff-file-path">{parsedDiff.Path}</span>
                                {(parsedDiff.Hunks ?? []).length > 0 && (
                                    <div className="view-toggle">
                                        <button
                                            className={!splitView ? "active" : ""}
                                            onClick={() => setSplitView(false)}
                                        >
                                            Unified
                                        </button>
                                        <button
                                            className={splitView ? "active" : ""}
                                            onClick={() => setSplitView(true)}
                                        >
                                            Split
                                        </button>
                                    </div>
                                )}
                            </div>
                        )}
                        {parsedDiff && (parsedDiff.Hunks ?? []).length === 0 && (
                            <p className="placeholder">No textual diff to display (untracked, binary, or rename-only)</p>
                        )}
                        {!splitView && parsedDiff && (parsedDiff.Hunks ?? []).map((hunk, hi) => (
                            <div key={hi} className="diff-hunk">
                                <div className="diff-hunk-header">{hunk.Header}</div>
                                {(hunk.Lines ?? []).map((line, li) => (
                                    <div key={li} className={`diff-line ${line.Type}`}>
                                        <span className="diff-gutter">{line.OldNum > 0 ? line.OldNum : ""}</span>
                                        <span className="diff-gutter">{line.NewNum > 0 ? line.NewNum : ""}</span>
                                        <span className="diff-sign">
                                            {line.Type === "added" ? "+" : line.Type === "removed" ? "-" : " "}
                                        </span>
                                        <span className="diff-content">{renderLineContent(line)}</span>
                                    </div>
                                ))}
                            </div>
                        ))}
                        {splitView && parsedDiff && (parsedDiff.Hunks ?? []).map((hunk, hi) => (
                            <div key={hi} className="diff-hunk">
                                <div className="diff-hunk-header">{hunk.Header}</div>
                                {splitRows(hunk.Lines ?? []).map((row, ri) => (
                                    <div key={ri} className="split-row">
                                        <div
                                            className={`split-cell split-cell-left${
                                                row.left
                                                    ? row.left.Type === "removed" ? " removed" : ""
                                                    : " split-filler"
                                            }`}
                                        >
                                            {row.left && (
                                                <>
                                                    <span className="diff-gutter">{row.left.OldNum > 0 ? row.left.OldNum : ""}</span>
                                                    <span className="diff-content">{renderLineContent(row.left)}</span>
                                                </>
                                            )}
                                        </div>
                                        <div
                                            className={`split-cell split-cell-right${
                                                row.right
                                                    ? row.right.Type === "added" ? " added" : ""
                                                    : " split-filler"
                                            }`}
                                        >
                                            {row.right && (
                                                <>
                                                    <span className="diff-gutter">{row.right.NewNum > 0 ? row.right.NewNum : ""}</span>
                                                    <span className="diff-content">{renderLineContent(row.right)}</span>
                                                </>
                                            )}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        ))}
                    </div>
                    {!explanationExpanded && (
                        <div className="pane-resizer" onMouseDown={startResize} />
                    )}
                    <div className="explanation-pane">
                        <div className="explanation-header">
                            <button
                                className="explanation-toggle-button"
                                onClick={() => setExplanationExpanded(!explanationExpanded)}
                            >
                                {explanationExpanded ? "⇤ Collapse" : "⇥ Expand"}
                            </button>
                            <button
                                className="explain-button"
                                disabled={!ready || checked.size === 0 || explaining}
                                onClick={explain}
                            >
                                {explaining ? "Explaining…" : "Explain"}
                            </button>
                        </div>
                        {checksList.length > 0 && (
                            <div className="check-buttons-row">
                                {checksList.map((check) => {
                                    const state = checkResults[check.Name];
                                    const disabled = !ready
                                        || checked.size === 0
                                        || (state?.running ?? false);
                                    return (
                                        <button
                                            key={check.Name}
                                            className="check-button"
                                            title={check.Description}
                                            disabled={disabled}
                                            onClick={() => runCheck(check)}
                                        >
                                            {state?.running ? `${check.Name}…` : check.Name}
                                        </button>
                                    );
                                })}
                            </div>
                        )}
                        {!explaining && !explanation && !explainError && (
                            <p className="placeholder">
                                {checked.size > 0
                                    ? "Click Explain to see an explanation of the checked files"
                                    : "Check one or more files, then click Explain"}
                            </p>
                        )}
                        {explaining && (
                            <p className="placeholder">Explaining…</p>
                        )}
                        {explainError && (
                            <p className="explain-error">{explainError}</p>
                        )}
                        {explanation && explainedCount > 1 && (
                            <p className="explanation-scope-label">{explainedCount} files</p>
                        )}
                        {explanation && (
                            <div className="markdown-body">
                                <ReactMarkdown>{explanation}</ReactMarkdown>
                            </div>
                        )}
                        {Object.keys(checkResults).length > 0 && (
                            <div className="check-results-box">
                                {Object.entries(checkResults).map(([name, state]) => (
                                    <div key={name} className="check-result">
                                        <p className="check-result-heading"><strong>{name}</strong></p>
                                        {state.running && (
                                            <p className="placeholder">Running…</p>
                                        )}
                                        {state.error && (
                                            <p className="explain-error">{state.error}</p>
                                        )}
                                        {!state.running && !state.error && state.result && (
                                            <div className="markdown-body">
                                                <ReactMarkdown>{state.result}</ReactMarkdown>
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>
                        )}
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
    const [verifying, setVerifying] = useState(false);
    const [verifyResult, setVerifyResult] = useState<"success" | string | null>(null);

    useEffect(() => {
        if (props.hasKey) {
            GetAPIKey().then(setNewKey);
        }
    }, [props.hasKey]);

    function save() {
        const cfg = new config.Config({base_url: baseURL, model});
        const tasks: Promise<unknown>[] = [SaveConfig(cfg)];
        if (newKey !== "") {
            tasks.push(SetAPIKey(newKey));
        }
        Promise.all(tasks).then(props.onClose);
    }

    function verify() {
        setVerifying(true);
        setVerifyResult(null);
        VerifyLLMConfig(baseURL, model, newKey)
            .then(() => setVerifyResult("success"))
            .catch((err) => setVerifyResult(String(err)))
            .finally(() => setVerifying(false));
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
                    <button
                        disabled={verifying || baseURL === "" || model === "" || (!props.hasKey && newKey === "")}
                        onClick={verify}
                    >
                        {verifying ? "Verifying…" : "Verify"}
                    </button>
                    <button onClick={props.onClose}>Cancel</button>
                    <button onClick={save}>Save</button>
                </div>
                {verifyResult === "success" && (
                    <p className="config-verify-success">✓ Connected successfully</p>
                )}
                {verifyResult && verifyResult !== "success" && (
                    <p className="explain-error">{verifyResult}</p>
                )}
            </div>
        </div>
    )
}

export default App
